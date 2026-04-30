package resources

import (
	"context"
	"fmt"

	"github.com/aviznetworks/ones-gfx-sdk/ones-gfx-sdk-go/ones_gfx"
)

// PeeringResource handles VPC peering operations.
type PeeringResource struct {
	transport *ones_gfx.Transport
}

// NewPeeringResource constructs a PeeringResource.
func NewPeeringResource(transport *ones_gfx.Transport) *PeeringResource {
	return &PeeringResource{transport: transport}
}

// Create creates a peering between two VPCs.
// Maps to POST /fabrics/{fabricName}/vpcpeering.
func (r *PeeringResource) Create(ctx context.Context, fabricName, name, vpcName, peerVPCName string) (interface{}, error) {
	if fabricName == "" {
		return nil, fmt.Errorf("fabricName is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if vpcName == "" {
		return nil, fmt.Errorf("vpcName is required")
	}
	if peerVPCName == "" {
		return nil, fmt.Errorf("peerVPCName is required")
	}

	body := map[string]string{
		"name":        name,
		"vpcname":     vpcName,
		"peervpcname": peerVPCName,
	}

	path := fmt.Sprintf("fabrics/%s/vpcpeering", fabricName)
	result, err := r.transport.Post(path, body, ones_gfx.OperationModeSynchronous, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}
