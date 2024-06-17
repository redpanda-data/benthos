package pure

import (
	"errors"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	if err := bloblang.RegisterMethodV2("mask",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryParsing).
			Description(`Masks a string using the given character, leaving X number of characters unmasked and returns a string.`).
			Param(bloblang.NewInt64Param("count").Description("the number of characters that will not be masked on the left or right hand side, in the case of a all mask, it is the number of mask characters to return giving a fixed length string, default is 0 which will return all characters masked.").Optional().Default(0)).
			Param(bloblang.NewStringParam("direction").Description("the direction to mask, left, right or all, default is all").Optional().Default("all")).
			Param(bloblang.NewStringParam("char").Description("the character used for masking, default is *").Optional().Default("*")).
			Example("Mask the first 13 characters", `root.body_mask = this.body.mask(13, "right")`,
				[2]string{
					`{"body":"the cat goes meow"}`,
					`{"body_mask":"*************meow"}`,
				},
			),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			countPtr, err := args.GetOptionalInt64("count")
			if err != nil {
				return nil, errors.New("failed to get count as int: " + err.Error())
			}
			count := int(*countPtr)

			char, err := args.GetString("char")
			if err != nil {
				return nil, errors.New("failed to get masking char as string: " + err.Error())
			}

			direction, err := args.GetString("direction")
			if err != nil {
				return nil, errors.New("failed to get direction as string: " + err.Error())
			}

			direction = strings.ToLower(direction)
			if direction != "left" && direction != "right" && direction != "all" {
				return nil, errors.New("direction must be one of left, right or all")
			}

			return bloblang.StringMethod(func(s string) (any, error) {
				return maskString(s, char, direction, int(count)), nil
			}), nil
		}); err != nil {
		panic(err)
	}
}

// maskString masks the string based on the given parameters.
func maskString(s string, char string, direction string, count int) string {
	sLength := len(s)
	if count == 0 {
		count = sLength
	}
	fixedMask := count
	if count > sLength {
		count = sLength
	}

	switch direction {
	case "left":
		unmasked := s[:count]
		masked := strings.Repeat(char, sLength-count)
		return unmasked + masked
	case "right":
		unmasked := s[sLength-count:]
		masked := strings.Repeat(char, sLength-count)
		return masked + unmasked
	default:
		masked := strings.Repeat(char, fixedMask)
		return masked
	}
}
