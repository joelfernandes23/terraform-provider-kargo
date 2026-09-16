package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestDurationSemanticEquality(t *testing.T) {
	ctx := context.Background()
	value := durationValue{StringValue: types.StringValue("10m")}
	for _, tc := range []struct {
		text  string
		equal bool
	}{{"10m0s", true}, {"600s", true}, {"11m", false}, {"invalid", false}} {
		equal, d := value.StringSemanticEquals(ctx, durationValue{StringValue: types.StringValue(tc.text)})
		if d.HasError() || equal != tc.equal {
			t.Errorf("%s equality=%t diagnostics=%v", tc.text, equal, d)
		}
	}
	if equal, _ := value.StringSemanticEquals(ctx, types.StringValue("10m")); equal {
		t.Error("unexpected type must not compare equal")
	}
	if equal, _ := value.StringSemanticEquals(ctx, durationValue{StringValue: types.StringNull()}); equal {
		t.Error("null must not compare equal")
	}
	typ := durationType{}
	if !typ.Equal(value.Type(ctx)) || typ.Equal(types.StringType) || typ.String() == "" {
		t.Error("duration type identity mismatch")
	}
	if !typ.ValueType(ctx).Equal(durationValue{}) {
		t.Error("incorrect value type")
	}
	if value.Equal(types.StringValue("10m")) {
		t.Error("different types compare equal")
	}
	converted, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(tftypes.String, "10m"))
	if err != nil || !converted.Equal(value) {
		t.Fatalf("conversion: %v %v", converted, err)
	}
	if _, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(tftypes.Number, 1)); err == nil {
		t.Error("number must not convert to duration")
	}
	convertedString, d := typ.ValueFromString(ctx, types.StringUnknown())
	if d.HasError() || !convertedString.IsUnknown() {
		t.Errorf("unknown conversion: %v %v", convertedString, d)
	}
}
