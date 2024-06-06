package pure

import (
	"fmt"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	if err := bloblang.RegisterMethodV2("mask",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryParsing).
			Description(`Masks a string using the given character, leaving X number of characters unmasked and returns a string.`).
			Param(bloblang.NewInt64Param("mask_count").Description("the number of characters that will not be masked on the left or right hand side, in the case of a all mask, it is the number of mask characters to return giving a fixed length string, default is 0 which will return all characters masked.").Optional().Default(0)).
			Param(bloblang.NewStringParam("direction").Description("the direction to mask, left, right or all, default is all").Optional().Default("all")).
			Param(bloblang.NewStringParam("mask_char").Description("the character used for masking, default is *").Optional().Default("*")).
			Example("", `root.values = this.body.mask(4)`,
				[2]string{
					`{"body":"noise=meow&animal=cat&fur=orange&fur=fluffy"}`,
					`{"values":{"animal":"cat","fur":["orange","fluffy"],"noise":"meow"}}`,
				},
			),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			maskCount, _ := args.GetOptionalInt64("mask_count")

			maskChar, _ := args.GetString("mask_char")

			direction, _ := args.GetString("direction")
			direction = strings.ToLower(direction)
			if direction != "left" && direction != "right" && direction != "all" {
				return nil, fmt.Errorf("direction must be one of left, right or all")
			}

			return bloblang.StringMethod(func(s string) (any, error) {
				return maskString(s, maskChar, direction, int(*maskCount)), nil
			}), nil
		}); err != nil {
		panic(err)
	}
}

// maskString masks the string based on the given parameters.
// `s` is the input string
// `maskChar` is the character used for masking,
// `direction` determines whether the start ("left") or the end ("right") of the string is masked or all for the whole string,
// `maskCount` is the number of characters that will not be masked on the left or right hand side, in the case of a all mask, it is the number of mask characters to return giving a fixed length string.
func maskString(s string, maskChar string, direction string, maskCount int) string {
	println("using maskChar: ", maskChar)
	sLength := int(len(s))
	if maskCount == 0 {
		maskCount = sLength
	}
	fixedMask := maskCount
	if maskCount > sLength {
		maskCount = sLength
	}

	switch direction {
	case "left":
		unmasked := s[:maskCount]
		masked := strings.Repeat(maskChar, sLength-maskCount)
		return unmasked + masked
	case "right":
		unmasked := s[sLength-maskCount:]
		masked := strings.Repeat(maskChar, sLength-maskCount)
		return masked + unmasked
	default:
		masked := strings.Repeat(maskChar, fixedMask)
		return masked
	}
}
