package resources

import (
	"context"
	"fmt"

	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx"
)

// FabricsResource handles operations on the fabrics collection.
type FabricsResource struct {
	transport *ones_gfx.Transport
}

// NewFabricsResource constructs a FabricsResource.
func NewFabricsResource(transport *ones_gfx.Transport) *FabricsResource {
	return &FabricsResource{transport: transport}
}

// List retrieves all fabrics defined in the ONES system.
// Maps to GET /fabrics.
func (r *FabricsResource) List(ctx context.Context) ([]ones_gfx.Fabric, error) {
	body, err := r.transport.Get("fabrics", nil)
	if err != nil {
		return nil, err
	}

	// Response shape: {"fabrics": [ {...}, {...} ]}
	obj, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", body)
	}

	fabricsRaw, ok := obj["fabrics"]
	if !ok {
		return nil, fmt.Errorf("response missing 'fabrics' field")
	}

	fabricsList, ok := fabricsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("'fabrics' field is not an array")
	}

	fabrics := make([]ones_gfx.Fabric, 0, len(fabricsList))
	for _, item := range fabricsList {
		jsonBytes, err := remarshal(item)
		if err != nil {
			continue
		}
		var fabric ones_gfx.Fabric
		if err := unmarshalJSON(jsonBytes, &fabric); err != nil {
			continue
		}
		fabrics = append(fabrics, fabric)
	}

	return fabrics, nil
}
