package storage

import (
	"context"

	proto "github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// PluginCSI wraps a CSI plugin and allows it to be used from inside nomad.
type PluginCSI struct {
	Logger log.Logger
	impl   *csiPluginClient
}

func (p *PluginCSI) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (CSIPlugin, error) {
	return &csiPluginClient{
		doneCtx: ctx,

		identityClient:   proto.NewIdentityClient(c),
		controllerClient: proto.NewControllerClient(c),
		nodeClient:       proto.NewNodeClient(c),

		logger: p.Logger.Named("csi"),
	}, nil
}
