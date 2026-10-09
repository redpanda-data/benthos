// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of the V1 AES methods. Both are deterministic given (scheme, key,
// iv, plaintext) so they sit with the pure plugins.

func init() {
	bloblangv2.MustRegisterMethod("encrypt_aes",
		bloblangv2.NewPluginSpec().
			Category("Encoding").
			Description("Encrypts a string or bytes value using AES under the named scheme and returns the ciphertext as bytes. Schemes: ctr, gcm, ofb, cbc.").
			Param(bloblangv2.NewStringParam("scheme").Description("AES scheme: ctr, gcm, ofb, or cbc.")).
			Param(bloblangv2.NewStringParam("key").Description("Encryption key. Length must match an AES variant: 16, 24, or 32 bytes.")).
			Param(bloblangv2.NewStringParam("iv").Description("Initialization vector / nonce.")),
		aesV2Ctor(true),
	)

	bloblangv2.MustRegisterMethod("decrypt_aes",
		bloblangv2.NewPluginSpec().
			Category("Encoding").
			Description("Decrypts a string or bytes ciphertext using AES under the named scheme and returns the plaintext as bytes. Schemes: ctr, gcm, ofb, cbc.").
			Param(bloblangv2.NewStringParam("scheme").Description("AES scheme: ctr, gcm, ofb, or cbc.")).
			Param(bloblangv2.NewStringParam("key").Description("Decryption key. Length must match an AES variant: 16, 24, or 32 bytes.")).
			Param(bloblangv2.NewStringParam("iv").Description("Initialization vector / nonce.")),
		aesV2Ctor(false),
	)
}

func aesV2Ctor(encrypt bool) bloblangv2.MethodConstructor {
	return func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
		scheme, err := args.GetString("scheme")
		if err != nil {
			return nil, err
		}
		key, err := args.GetString("key")
		if err != nil {
			return nil, err
		}
		iv, err := args.GetString("iv")
		if err != nil {
			return nil, err
		}

		block, err := aes.NewCipher([]byte(key))
		if err != nil {
			return nil, err
		}
		ivBytes := []byte(iv)
		switch scheme {
		case "ctr", "ofb", "cbc":
			if len(ivBytes) != block.BlockSize() {
				return nil, errors.New("the iv length must match the AES block size")
			}
		}

		var fn func([]byte) ([]byte, error)
		if encrypt {
			fn, err = buildAESEncrypt(scheme, block, ivBytes)
		} else {
			fn, err = buildAESDecrypt(scheme, block, ivBytes)
		}
		if err != nil {
			return nil, err
		}

		return func(v any) (any, error) {
			switch t := v.(type) {
			case string:
				return fn([]byte(t))
			case []byte:
				return fn(t)
			}
			return nil, fmt.Errorf("expected string or bytes receiver, got %T", v)
		}, nil
	}
}

func buildAESEncrypt(scheme string, block cipher.Block, iv []byte) (func([]byte) ([]byte, error), error) {
	switch scheme {
	case "ctr":
		return func(b []byte) ([]byte, error) {
			out := make([]byte, len(b))
			cipher.NewCTR(block, iv).XORKeyStream(out, b)
			return out, nil
		}, nil
	case "gcm":
		return func(b []byte) ([]byte, error) {
			s, err := cipher.NewGCM(block)
			if err != nil {
				return nil, fmt.Errorf("creating gcm failed: %w", err)
			}
			return s.Seal(nil, iv, b, nil), nil
		}, nil
	case "ofb":
		return func(b []byte) ([]byte, error) {
			out := make([]byte, len(b))
			//nolint:staticcheck // SA1019: cipher.NewOFB has been deprecated since Go 1.24
			cipher.NewOFB(block, iv).XORKeyStream(out, b)
			return out, nil
		}, nil
	case "cbc":
		return func(b []byte) ([]byte, error) {
			if len(b)%aes.BlockSize != 0 {
				return nil, errors.New("plaintext is not a multiple of the block size")
			}
			out := make([]byte, len(b))
			cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, b)
			return out, nil
		}, nil
	}
	return nil, fmt.Errorf("unrecognized encryption scheme: %v", scheme)
}

func buildAESDecrypt(scheme string, block cipher.Block, iv []byte) (func([]byte) ([]byte, error), error) {
	switch scheme {
	case "ctr":
		return func(b []byte) ([]byte, error) {
			out := make([]byte, len(b))
			cipher.NewCTR(block, iv).XORKeyStream(out, b)
			return out, nil
		}, nil
	case "gcm":
		return func(b []byte) ([]byte, error) {
			s, err := cipher.NewGCM(block)
			if err != nil {
				return nil, fmt.Errorf("creating gcm failed: %w", err)
			}
			out, err := s.Open(nil, iv, b, nil)
			if err != nil {
				return nil, fmt.Errorf("gcm decrypting failed: %w", err)
			}
			return out, nil
		}, nil
	case "ofb":
		return func(b []byte) ([]byte, error) {
			out := make([]byte, len(b))
			//nolint:staticcheck // SA1019: cipher.NewOFB has been deprecated since Go 1.24
			cipher.NewOFB(block, iv).XORKeyStream(out, b)
			return out, nil
		}, nil
	case "cbc":
		return func(b []byte) ([]byte, error) {
			if len(b)%aes.BlockSize != 0 {
				return nil, errors.New("ciphertext is not a multiple of the block size")
			}
			out := make([]byte, len(b))
			cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, b)
			return out, nil
		}, nil
	}
	return nil, fmt.Errorf("unrecognized decryption scheme: %v", scheme)
}
