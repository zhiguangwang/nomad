package storage

import (
	"context"

	proto "github.com/container-storage-interface/spec/lib/go/csi"
	hclog "github.com/hashicorp/go-hclog"
)

type csiPluginClient struct {
	doneCtx context.Context

	identityClient   proto.IdentityClient
	controllerClient proto.ControllerClient
	nodeClient       proto.NodeClient

	logger hclog.Logger
}

func (c *csiPluginClient) Fingerprint(ctx context.Context) (chan<- *FingerprintResponse, error) {
	return nil, nil
}
