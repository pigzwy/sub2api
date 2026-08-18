package repository

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// s3ResponseError fabricates the transport-level error shape the AWS SDK
// returns when a service answers with a bare HTTP status (HeadBucket has no
// body, so classification can only look at the status code).
func s3ResponseError(status int) error {
	return &awshttp.ResponseError{ResponseError: &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
		Err:      errors.New("api error"),
	}}
}

func TestClassifyS3ProbeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"timeout maps to unreachable", context.DeadlineExceeded, service.ErrVideoStorageUnreachable},
		{"canceled maps to unreachable", context.Canceled, service.ErrVideoStorageUnreachable},
		{"NoSuchBucket code", &smithy.GenericAPIError{Code: "NoSuchBucket"}, service.ErrVideoStorageBucketNotFound},
		{"bare HTTP 404", s3ResponseError(http.StatusNotFound), service.ErrVideoStorageBucketNotFound},
		{"SignatureDoesNotMatch", &smithy.GenericAPIError{Code: "SignatureDoesNotMatch"}, service.ErrVideoStorageAccessDenied},
		{"InvalidAccessKeyId", &smithy.GenericAPIError{Code: "InvalidAccessKeyId"}, service.ErrVideoStorageAccessDenied},
		{"bare HTTP 403", s3ResponseError(http.StatusForbidden), service.ErrVideoStorageAccessDenied},
		{"DNS failure", &net.DNSError{Err: "no such host", Name: "r2.example"}, service.ErrVideoStorageUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, classifyS3ProbeError("probe", "bucket", tc.err), tc.want)
		})
	}

	require.NoError(t, classifyS3ProbeError("probe", "bucket", nil))

	// An unrecognized code stays unclassified so the UI falls back to the raw
	// message, and the summary keeps the code readable instead of the SDK dump.
	got := classifyS3ProbeError("probe", "bucket", &smithy.GenericAPIError{Code: "SlowDown"})
	for _, sentinel := range []error{
		service.ErrVideoStorageBucketNotFound,
		service.ErrVideoStorageAccessDenied,
		service.ErrVideoStorageUnreachable,
	} {
		require.NotErrorIs(t, got, sentinel)
	}
	require.Contains(t, got.Error(), "SlowDown")
}
