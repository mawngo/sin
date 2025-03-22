package utils

import (
	"crypto/sha256"
	"github.com/mitchellh/mapstructure"
	"io"
	"os"
)

func MapToStruct(m map[string]any, s any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		Squash:  true,
		// From viper.Unmarshal for consistency.
		Metadata:         nil,
		Result:           &s,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return err
	}
	return decoder.Decode(m)
}

func FileSHA256Checksum(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
