package resources

import (
	"context"
	"fmt"

	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx"
)

// OperationsResource handles polling for async operations.
type OperationsResource struct {
	transport *ones_gfx.Transport
}

// NewOperationsResource constructs an OperationsResource.
func NewOperationsResource(transport *ones_gfx.Transport) *OperationsResource {
	return &OperationsResource{transport: transport}
}

// Get fetches the current state of an async operation.
// Maps to GET /operations/{operationId}.
func (r *OperationsResource) Get(ctx context.Context, operationID string) (*ones_gfx.Operation, error) {
	if operationID == "" {
		return nil, fmt.Errorf("operationID cannot be empty")
	}

	path := fmt.Sprintf("operations/%s", operationID)
	body, err := r.transport.Get(path, nil)
	if err != nil {
		return nil, err
	}

	jsonBytes, err := remarshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to remarshal operation: %w", err)
	}

	var op ones_gfx.Operation
	if err := unmarshalJSON(jsonBytes, &op); err != nil {
		return nil, fmt.Errorf("failed to unmarshal operation: %w", err)
	}

	return &op, nil
}
