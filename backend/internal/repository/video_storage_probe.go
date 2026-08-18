package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// videoStorageProbeObject 是"测试连接"写入的一次性对象名，落在配置的 Key 前缀下。
const videoStorageProbeObject = ".connection-test"

var videoStorageProbeBody = []byte("sub2api video storage connection test")

var _ service.VideoStorageProbe = (*S3ImageStorage)(nil)

// ProbeVideoStorage 真实验证桶可用：HeadBucket → 写入测试对象 → 删除。
//
// 只构造客户端不发任何请求时，桶名写错、凭据无效、endpoint 不可达都会显示"连接成功"，
// 这里用一次真实往返把这三类问题区分出来。
func (s *S3ImageStorage) ProbeVideoStorage(ctx context.Context, prefix string) error {
	// HeadBucket 能廉价地回答"桶名/endpoint 是否正确"，但部分只授予对象读写的令牌
	// （Cloudflare R2、MinIO 自定义策略）会拒绝它却允许写入。因此只有它给出的确定性
	// 结论才中断探测，其余情况交给下面的写入来定论。
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket}); err != nil {
		classified := classifyS3ProbeError("head bucket", s.bucket, err)
		if errors.Is(classified, service.ErrVideoStorageBucketNotFound) ||
			errors.Is(classified, service.ErrVideoStorageUnreachable) {
			return classified
		}
	}

	key := strings.TrimPrefix(prefix, "/") + videoStorageProbeObject
	contentType := "text/plain"
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &key,
		Body:        bytes.NewReader(videoStorageProbeBody),
		ContentType: &contentType,
	}); err != nil {
		return classifyS3ProbeError("write test object "+key, s.bucket, err)
	}

	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	}); err != nil {
		// 写入已经成功，措辞必须说清楚：只是清理失败，不要让人误以为不能存视频。
		return classifyS3ProbeError(
			"write succeeded but cleanup of test object "+key+" failed", s.bucket, err)
	}
	return nil
}

// classifyS3ProbeError 把冗长的 SDK 错误归类成后台能本地化的三类原因。
func classifyS3ProbeError(op, bucket string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return fmt.Errorf("%w: %s timed out", service.ErrVideoStorageUnreachable, op)
	case isS3BucketMissing(err):
		return fmt.Errorf("%w: %s (bucket %q): %s",
			service.ErrVideoStorageBucketNotFound, op, bucket, summarizeS3Error(err))
	case isS3AccessDenied(err):
		return fmt.Errorf("%w: %s (bucket %q): %s",
			service.ErrVideoStorageAccessDenied, op, bucket, summarizeS3Error(err))
	case isS3Unreachable(err):
		return fmt.Errorf("%w: %s: %s", service.ErrVideoStorageUnreachable, op, summarizeS3Error(err))
	}
	return fmt.Errorf("%s (bucket %q): %s", op, bucket, summarizeS3Error(err))
}

// isS3BucketMissing 判断桶不存在。HeadBucket 的响应没有 body，多数 S3 兼容服务
// 只能靠 404 状态码识别。
func isS3BucketMissing(err error) bool {
	var noSuchBucket *types.NoSuchBucket
	var notFound *types.NotFound
	if errors.As(err, &noSuchBucket) || errors.As(err, &notFound) {
		return true
	}
	switch s3ErrorCode(err) {
	case "NoSuchBucket", "NotFound":
		return true
	}
	return s3HTTPStatus(err) == http.StatusNotFound
}

// isS3AccessDenied 覆盖 AWS S3 / Cloudflare R2 / MinIO / 阿里云 OSS 的凭据与权限错误码。
func isS3AccessDenied(err error) bool {
	switch s3ErrorCode(err) {
	case "AccessDenied", "AccessDeniedException", "AccountProblem", "AllAccessDisabled",
		"ExpiredToken", "Forbidden", "InvalidAccessKeyId", "InvalidSecurity",
		"SignatureDoesNotMatch", "Unauthorized":
		return true
	}
	switch s3HTTPStatus(err) {
	case http.StatusUnauthorized, http.StatusForbidden:
		return true
	}
	return false
}

// isS3Unreachable 判断请求根本没拿到 HTTP 响应：DNS、TCP、TLS 或 endpoint 填错。
func isS3Unreachable(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return s3HTTPStatus(err) == 0 && s3ErrorCode(err) == ""
}

func s3ErrorCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

func s3HTTPStatus(err error) int {
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) {
		return respErr.HTTPStatusCode()
	}
	return 0
}

// summarizeS3Error 把 SDK 错误压缩成 "Code (HTTP 403)" 这样的短说明，
// 便于直接显示在后台，而不是甩一整段调用链。
func summarizeS3Error(err error) string {
	parts := make([]string, 0, 2)
	if code := s3ErrorCode(err); code != "" {
		parts = append(parts, code)
	}
	if status := s3HTTPStatus(err); status != 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", status))
	}
	if len(parts) == 0 {
		return err.Error()
	}
	return strings.Join(parts, ", ")
}
