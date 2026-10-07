package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestReadOpenAIWSClientMessageRejectsAmbiguousSessionFrames(t *testing.T) {
	for _, payload := range []string{
		`{"type":"session.update","session":{"model":"a","Model":"b"}}`,
		`{"type":"session.update","session":{},"Session":{"model":"b"}}`,
		`{"type":"response.create","model":"a","\u006dodel":"a"}`,
	} {
		for _, messageType := range []coderws.MessageType{coderws.MessageText, coderws.MessageBinary} {
			t.Run(payload+messageType.String(), func(t *testing.T) {
				result := make(chan error, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, err := coderws.Accept(w, r, nil)
					if err != nil {
						result <- err
						return
					}
					defer func() { _ = conn.CloseNow() }()
					_, body, err := ReadOpenAIWSClientMessage(r.Context(), conn, time.Second, coderws.StatusPolicyViolation, "timeout")
					if len(body) > 0 {
						result <- errors.New("ambiguous payload exposed to forwarding caller")
						return
					}
					result <- err
				}))
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
				require.NoError(t, err)
				defer func() { _ = client.CloseNow() }()
				require.NoError(t, client.Write(ctx, messageType, []byte(payload)))
				_, _, err = client.Read(ctx)
				require.Equal(t, coderws.StatusPolicyViolation, coderws.CloseStatus(err))
				select {
				case err := <-result:
					var closeErr *OpenAIWSClientCloseError
					require.ErrorAs(t, err, &closeErr)
					require.Equal(t, requestmodel.AmbiguousModelMessage, closeErr.Reason())
				case <-ctx.Done():
					t.Fatal("reader did not exit")
				}
			})
		}
	}
}

func TestReadOpenAIWSClientMessage_ControlCloseFrames(t *testing.T) {
	tests := []struct {
		name          string
		timeout       time.Duration
		timeoutStatus coderws.StatusCode
		timeoutReason string
		cancelCause   error
		wantStatus    coderws.StatusCode
		wantReason    string
	}{
		{
			name:          "inter-turn idle sends normal close",
			timeout:       25 * time.Millisecond,
			timeoutStatus: coderws.StatusNormalClosure,
			timeoutReason: "websocket idle timeout",
			wantStatus:    coderws.StatusNormalClosure,
			wantReason:    "websocket idle timeout",
		},
		{
			name:          "first message timeout sends policy close",
			timeout:       25 * time.Millisecond,
			timeoutStatus: coderws.StatusPolicyViolation,
			timeoutReason: "missing first response.create message",
			wantStatus:    coderws.StatusPolicyViolation,
			wantReason:    "missing first response.create message",
		},
		{
			name:        "lease loss sends retry close",
			cancelCause: ErrOpenAIWSIngressLeaseLost,
			wantStatus:  coderws.StatusTryAgainLater,
			wantReason:  "websocket ingress capacity lease lost; please reconnect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controlCtx, cancelControl := context.WithCancelCause(context.Background())
			defer cancelControl(context.Canceled)
			serverResult := make(chan error, 1)
			readStarted := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverResult <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				close(readStarted)
				_, _, err = ReadOpenAIWSClientMessage(
					controlCtx,
					conn,
					tt.timeout,
					tt.timeoutStatus,
					tt.timeoutReason,
				)
				serverResult <- err
			}))
			defer server.Close()

			dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
			clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			cancelDial()
			require.NoError(t, err)
			defer func() { _ = clientConn.CloseNow() }()
			<-readStarted
			if tt.cancelCause != nil {
				cancelControl(tt.cancelCause)
			}

			readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
			_, _, err = clientConn.Read(readCtx)
			cancelRead()
			var clientClose coderws.CloseError
			require.ErrorAs(t, err, &clientClose)
			require.Equal(t, tt.wantStatus, clientClose.Code)
			require.Equal(t, tt.wantReason, clientClose.Reason)

			select {
			case serverErr := <-serverResult:
				var closeErr *OpenAIWSClientCloseError
				require.ErrorAs(t, serverErr, &closeErr)
				require.Equal(t, tt.wantStatus, closeErr.StatusCode())
				require.Equal(t, tt.wantReason, closeErr.Reason())
			case <-time.After(time.Second):
				t.Fatal("server read goroutine did not exit after close handshake")
			}
		})
	}
}

func TestReadOpenAIWSClientMessage_ParentCancellationStillJoinsRead(t *testing.T) {
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	serverResult := make(chan error, 1)
	readStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverResult <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		close(readStarted)
		_, _, err = ReadOpenAIWSClientMessage(controlCtx, conn, 0, 0, "")
		serverResult <- err
	}))
	defer server.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()
	<-readStarted
	cancelControl(errors.New("server shutting down"))
	readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	var clientClose coderws.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, coderws.StatusGoingAway, clientClose.Code)
	require.Equal(t, "websocket request canceled", clientClose.Reason)

	select {
	case <-serverResult:
	case <-time.After(time.Second):
		t.Fatal("server read goroutine leaked after parent cancellation")
	}
}
