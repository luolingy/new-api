package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/s3_setting"
)

// MaxAvatarUploadBytes is the hard upper bound for an avatar upload. The
// browser crops and re-encodes before sending, so a larger body is either a
// mistake or an attempt to exhaust memory.
const MaxAvatarUploadBytes = 1 << 20

// MaxAvatarDimension bounds decoded images so a tiny file header cannot force a
// huge allocation during validation.
const MaxAvatarDimension = 8192

// ValidateAvatarImage checks the payload is a decodable JPEG or PNG image and
// returns its MIME type and file extension.
func ValidateAvatarImage(data []byte) (string, string, error) {
	contentType := http.DetectContentType(data)
	if contentType != "image/jpeg" && contentType != "image/png" {
		return "", "", errors.New("头像必须是 JPEG 或 PNG 图片")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", "", errors.New("头像图片无法解析")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", "", errors.New("头像图片尺寸无效")
	}
	if cfg.Width > MaxAvatarDimension || cfg.Height > MaxAvatarDimension {
		return "", "", fmt.Errorf("头像图片尺寸不能超过 %d 像素", MaxAvatarDimension)
	}
	if contentType == "image/png" {
		return contentType, "png", nil
	}
	return contentType, "jpg", nil
}

var emptySHA256 = hexSHA256(nil)

var s3HTTPClient = &http.Client{Timeout: 30 * time.Second}

// s3Target is the resolved request target for one object key.
type s3Target struct {
	requestURL   string
	host         string
	canonicalURI string
}

// NewAvatarObjectKey builds a unique object key for a user avatar.
func NewAvatarObjectKey(userID int, ext string) string {
	return fmt.Sprintf("%s/%d/%d-%s.%s", s3_setting.GetS3Setting().NormalizedPrefix(), userID, time.Now().Unix(), randomHex(4), ext)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// UploadAvatarObject stores one avatar image and returns its public URL.
func UploadAvatarObject(ctx context.Context, key, contentType string, data []byte) (string, error) {
	cfg := s3_setting.GetS3Setting()
	if !cfg.Ready() {
		return "", errors.New("尚未启用或配置 S3 存储，无法上传头像")
	}
	if len(data) == 0 || len(data) > MaxAvatarUploadBytes {
		return "", fmt.Errorf("头像文件大小必须在 1 字节到 %d 字节之间", MaxAvatarUploadBytes)
	}
	target, err := buildS3Target(cfg, key)
	if err != nil {
		return "", err
	}
	payloadHash := hexSHA256(data)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.requestURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Host = target.host
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", contentType)
	signS3Request(req, target, payloadHash, cfg, time.Now())
	resp, err := s3HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传头像到 S3 失败: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("S3 返回状态码 %d，请检查存储配置", resp.StatusCode)
	}
	return PublicObjectURL(cfg, key)
}

// DeleteAvatarObject removes one object. A missing object is not an error.
func DeleteAvatarObject(ctx context.Context, key string) error {
	cfg := s3_setting.GetS3Setting()
	if !cfg.Configured() || strings.TrimSpace(key) == "" {
		return nil
	}
	target, err := buildS3Target(cfg, key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target.requestURL, nil)
	if err != nil {
		return err
	}
	req.Host = target.host
	signS3Request(req, target, emptySHA256, cfg, time.Now())
	resp, err := s3HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("S3 返回状态码 %d", resp.StatusCode)
	}
	return nil
}

// DeleteAvatarByURL removes a previously stored avatar when the URL still
// points at the configured bucket. Unknown URLs are ignored so that avatars
// imported from OAuth providers never trigger remote deletions.
func DeleteAvatarByURL(ctx context.Context, rawURL string) {
	cfg := s3_setting.GetS3Setting()
	if !cfg.Configured() {
		return
	}
	key, ok := objectKeyFromURL(cfg, rawURL)
	if !ok {
		return
	}
	_ = DeleteAvatarObject(ctx, key)
}

// TestS3Connection verifies the endpoint and credentials with a bucket HEAD.
func TestS3Connection(ctx context.Context) error {
	cfg := s3_setting.GetS3Setting()
	if !cfg.Configured() {
		return errors.New("请先填写 S3 Endpoint、Bucket、Access Key 和 Secret Key")
	}
	target, err := buildBucketTarget(cfg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target.requestURL, nil)
	if err != nil {
		return err
	}
	req.Host = target.host
	signS3Request(req, target, emptySHA256, cfg, time.Now())
	resp, err := s3HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接 S3 Endpoint: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusUnauthorized:
		return errors.New("S3 鉴权失败，请检查 Access Key / Secret Key 与 Bucket 权限")
	case resp.StatusCode == http.StatusNotFound:
		return errors.New("Bucket 不存在，请检查 Bucket 名称")
	default:
		return fmt.Errorf("S3 返回状态码 %d", resp.StatusCode)
	}
}

// PublicObjectURL joins the configured public base (or the endpoint) with an
// object key.
func PublicObjectURL(cfg *s3_setting.S3Setting, key string) (string, error) {
	escaped := escapeObjectKey(key)
	base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	if base == "" {
		if cfg.ForcePathStyle {
			base = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/") + "/" + cfg.Bucket
		} else {
			endpoint, err := url.Parse(cfg.Endpoint)
			if err != nil {
				return "", fmt.Errorf("S3 Endpoint 无效: %w", err)
			}
			base = endpoint.Scheme + "://" + cfg.Bucket + "." + endpoint.Host
		}
	}
	return base + "/" + escaped, nil
}

func objectKeyFromURL(cfg *s3_setting.S3Setting, rawURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", false
	}
	bases := []string{strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")}
	if endpointBase, err := PublicObjectURL(&s3_setting.S3Setting{
		Endpoint:       cfg.Endpoint,
		Bucket:         cfg.Bucket,
		ForcePathStyle: cfg.ForcePathStyle,
	}, ""); err == nil {
		bases = append(bases, strings.TrimRight(endpointBase, "/"))
	}
	for _, base := range bases {
		if base == "" {
			continue
		}
		if !strings.HasPrefix(rawURL, base+"/") {
			continue
		}
		key, err := url.PathUnescape(strings.TrimPrefix(rawURL, base+"/"))
		if err != nil || key == "" {
			return "", false
		}
		return key, true
	}
	return "", false
}

func buildS3Target(cfg *s3_setting.S3Setting, key string) (*s3Target, error) {
	endpoint, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil || endpoint.Host == "" {
		return nil, fmt.Errorf("S3 Endpoint 无效")
	}
	basePath := strings.TrimRight(endpoint.EscapedPath(), "/")
	escapedKey := escapeObjectKey(key)
	host := endpoint.Host
	uri := basePath
	if cfg.ForcePathStyle {
		uri += "/" + escapeObjectKey(cfg.Bucket)
	} else {
		host = cfg.Bucket + "." + endpoint.Host
	}
	uri += "/" + escapedKey
	return &s3Target{
		requestURL:   endpoint.Scheme + "://" + host + uri,
		host:         host,
		canonicalURI: uri,
	}, nil
}

func buildBucketTarget(cfg *s3_setting.S3Setting) (*s3Target, error) {
	endpoint, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil || endpoint.Host == "" {
		return nil, fmt.Errorf("S3 Endpoint 无效")
	}
	basePath := strings.TrimRight(endpoint.EscapedPath(), "/")
	host := endpoint.Host
	uri := basePath + "/" + escapeObjectKey(cfg.Bucket)
	if !cfg.ForcePathStyle {
		host = cfg.Bucket + "." + endpoint.Host
		uri = basePath + "/"
	}
	return &s3Target{
		requestURL:   endpoint.Scheme + "://" + host + uri,
		host:         host,
		canonicalURI: uri,
	}, nil
}

// escapeObjectKey percent-encodes every object key segment while keeping the
// path separators that S3 expects.
func escapeObjectKey(key string) string {
	segments := strings.Split(strings.TrimPrefix(key, "/"), "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// signS3Request adds AWS Signature Version 4 headers for the S3 service.
// Only host, x-amz-content-sha256 and x-amz-date are signed, which matches how
// S3 evaluates presigned-less requests with unsigned payload headers.
func signS3Request(req *http.Request, target *s3Target, payloadHash string, cfg *s3_setting.S3Setting, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := amzDate[:8]
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	canonicalHeaders := "host:" + target.host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		req.Method,
		target.canonicalURI,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(cfg.SecretKey, dateStamp, region, "s3"), []byte(stringToSign)))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+cfg.AccessKey+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func signingKey(secretKey, dateStamp, region, service string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	regionKey := hmacSHA256(dateKey, []byte(region))
	serviceKey := hmacSHA256(regionKey, []byte(service))
	return hmacSHA256(serviceKey, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
