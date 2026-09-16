package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// durationType preserves configured spelling when Kargo normalizes a duration.
type durationType struct{ basetypes.StringType }
type durationValue struct{ basetypes.StringValue }

func (durationType) String() string                       { return "durationType" }
func (durationType) ValueType(context.Context) attr.Value { return durationValue{} }
func (durationType) Equal(other attr.Type) bool           { _, ok := other.(durationType); return ok }
func (durationType) ValueFromString(_ context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return durationValue{StringValue: value}, nil
}
func (t durationType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
	converted, err := t.StringType.ValueFromTerraform(ctx, value)
	if err != nil {
		return nil, err
	}
	return durationValue{StringValue: converted.(basetypes.StringValue)}, nil
}
func (durationValue) Type(context.Context) attr.Type { return durationType{} }
func (v durationValue) Equal(other attr.Value) bool {
	w, ok := other.(durationValue)
	return ok && v.StringValue.Equal(w.StringValue)
}
func (v durationValue) StringSemanticEquals(_ context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	w, ok := other.(durationValue)
	if !ok || v.IsNull() || v.IsUnknown() || w.IsNull() || w.IsUnknown() {
		return false, nil
	}
	a, errA := time.ParseDuration(v.ValueString())
	b, errB := time.ParseDuration(w.ValueString())
	return errA == nil && errB == nil && a == b, nil
}
