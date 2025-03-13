package utils

import "github.com/mitchellh/mapstructure"

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
