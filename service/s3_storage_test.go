package service

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/s3_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodeTestImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	buf := new(bytes.Buffer)
	var err error
	if format == "png" {
		err = png.Encode(buf, img)
	} else {
		err = jpeg.Encode(buf, img, nil)
	}
	require.NoError(t, err)
	return buf.Bytes()
}

func TestValidateAvatarImage(t *testing.T) {
	t.Run("accepts png and jpeg", func(t *testing.T) {
		contentType, ext, err := ValidateAvatarImage(encodeTestImage(t, "png", 256, 256))
		require.NoError(t, err)
		assert.Equal(t, "image/png", contentType)
		assert.Equal(t, "png", ext)

		contentType, ext, err = ValidateAvatarImage(encodeTestImage(t, "jpeg", 128, 128))
		require.NoError(t, err)
		assert.Equal(t, "image/jpeg", contentType)
		assert.Equal(t, "jpg", ext)
	})

	t.Run("rejects non images", func(t *testing.T) {
		_, _, err := ValidateAvatarImage([]byte("not an image at all"))
		require.Error(t, err)
	})

	t.Run("rejects oversized dimensions", func(t *testing.T) {
		_, _, err := ValidateAvatarImage(encodeTestImage(t, "png", MaxAvatarDimension+1, 1))
		require.Error(t, err)
	})
}

func TestPublicObjectURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  *s3_setting.S3Setting
		key  string
		want string
	}{
		{
			name: "public base url wins over endpoint",
			cfg: &s3_setting.S3Setting{
				Endpoint:       "https://s3.internal.example.com",
				PublicBaseURL:  "https://cdn.example.com/media/",
				Bucket:         "avatars",
				ForcePathStyle: true,
			},
			key:  "avatars/1/x.png",
			want: "https://cdn.example.com/media/avatars/1/x.png",
		},
		{
			name: "path style endpoint",
			cfg: &s3_setting.S3Setting{
				Endpoint:       "https://s3.example.com",
				Bucket:         "bucket",
				ForcePathStyle: true,
			},
			key:  "avatars/1/x.png",
			want: "https://s3.example.com/bucket/avatars/1/x.png",
		},
		{
			name: "virtual host style",
			cfg: &s3_setting.S3Setting{
				Endpoint:       "https://s3.example.com",
				Bucket:         "bucket",
				ForcePathStyle: false,
			},
			key:  "avatars/1/x.png",
			want: "https://bucket.s3.example.com/avatars/1/x.png",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PublicObjectURL(tc.cfg, tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestObjectKeyFromURL(t *testing.T) {
	cfg := &s3_setting.S3Setting{
		Endpoint:       "https://s3.example.com",
		PublicBaseURL:  "https://cdn.example.com",
		Bucket:         "bucket",
		ForcePathStyle: true,
	}
	key, ok := objectKeyFromURL(cfg, "https://cdn.example.com/avatars/7/a.png")
	require.True(t, ok)
	assert.Equal(t, "avatars/7/a.png", key)

	// Endpoint URLs (no separate public base) still resolve.
	key, ok = objectKeyFromURL(cfg, "https://s3.example.com/bucket/avatars/7/b.png")
	require.True(t, ok)
	assert.Equal(t, "avatars/7/b.png", key)

	_, ok = objectKeyFromURL(cfg, "https://other.example.com/avatars/7/a.png")
	assert.False(t, ok)
}

func TestEscapeObjectKey(t *testing.T) {
	assert.Equal(t, "avatars/1/a%20b.png", escapeObjectKey("avatars/1/a b.png"))
	assert.Equal(t, "avatars/%E4%B8%AD%E6%96%87.png", escapeObjectKey("/avatars/中文.png"))
}

func TestSignS3RequestIsStable(t *testing.T) {
	cfg := &s3_setting.S3Setting{
		Endpoint:       "https://s3.example.com",
		Region:         "us-east-1",
		Bucket:         "bucket",
		ForcePathStyle: true,
		AccessKey:      "AKIAEXAMPLE",
		SecretKey:      "secret",
	}
	target, err := buildS3Target(cfg, "avatars/1/a.png")
	require.NoError(t, err)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	newRequest := func() *http.Request {
		req, err := http.NewRequest(http.MethodPut, target.requestURL, nil)
		require.NoError(t, err)
		req.Host = target.host
		return req
	}

	first := newRequest()
	signS3Request(first, target, hexSHA256([]byte("payload")), cfg, now)
	second := newRequest()
	signS3Request(second, target, hexSHA256([]byte("payload")), cfg, now)

	auth := first.Header.Get("Authorization")
	assert.Equal(t, second.Header.Get("Authorization"), auth)
	assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/20260102/us-east-1/s3/aws4_request"))
	assert.Contains(t, auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date")
	assert.Equal(t, "20260102T030405Z", first.Header.Get("X-Amz-Date"))

	// A different payload must change the signature.
	other := newRequest()
	signS3Request(other, target, hexSHA256([]byte("other")), cfg, now)
	assert.NotEqual(t, auth, other.Header.Get("Authorization"))
}

// TestSigningKeyMatchesAWSDocumentation checks the derived signing key against
// the example published in the AWS Signature Version 4 documentation.
func TestSigningKeyMatchesAWSDocumentation(t *testing.T) {
	got := signingKey("wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "20120215", "us-east-1", "iam")
	assert.Equal(
		t,
		"f4780e2d9f65fa895f9c67b32ce1baf0b0d8a43505a000a1a9e090d414db404d",
		hex.EncodeToString(got),
	)
}

func TestS3SettingReadiness(t *testing.T) {
	empty := &s3_setting.S3Setting{}
	assert.False(t, empty.Configured())

	partial := &s3_setting.S3Setting{Endpoint: "https://s3.example.com", Bucket: "bucket"}
	assert.False(t, partial.Configured(), "credentials are required to sign requests")

	configured := &s3_setting.S3Setting{
		Endpoint:  "https://s3.example.com",
		Bucket:    "bucket",
		AccessKey: "ak",
		SecretKey: "sk",
	}
	assert.True(t, configured.Configured())
	assert.False(t, configured.Ready(), "uploads must stay off until enabled")

	configured.Enabled = true
	assert.True(t, configured.Ready())

	assert.Equal(t, "avatars", empty.NormalizedPrefix())
	assert.Equal(t, "custom/path", (&s3_setting.S3Setting{PathPrefix: "/custom/path/"}).NormalizedPrefix())
}

func TestS3SettingValidation(t *testing.T) {
	require.NoError(t, s3_setting.ValidateEndpoint(""))
	require.NoError(t, s3_setting.ValidateEndpoint("https://s3.example.com"))
	require.Error(t, s3_setting.ValidateEndpoint("ftp://s3.example.com"))
	require.Error(t, s3_setting.ValidateEndpoint("https://user:pass@s3.example.com"))

	require.NoError(t, s3_setting.ValidatePublicBaseURL("https://cdn.example.com/assets"))
	require.Error(t, s3_setting.ValidatePublicBaseURL("/relative"))

	require.NoError(t, s3_setting.ValidateBucket("my-bucket.1"))
	require.Error(t, s3_setting.ValidateBucket("Bad_Bucket"))

	require.NoError(t, s3_setting.ValidatePathPrefix("avatars/2026"))
	require.Error(t, s3_setting.ValidatePathPrefix("../escape"))
	require.Error(t, s3_setting.ValidatePathPrefix("/leading"))

	require.NoError(t, s3_setting.ValidateRegion("us-east-1"))
	require.Error(t, s3_setting.ValidateRegion("us east"))
}
