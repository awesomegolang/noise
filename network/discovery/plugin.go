package discovery

import (
	"context"

	"github.com/perlin-network/noise/internal/protobuf"
	"github.com/perlin-network/noise/log"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/peer"

	"github.com/pkg/errors"
)

type Plugin struct {
	*network.Plugin

	DisablePing   bool
	DisablePong   bool
	DisableLookup bool

	// EnforceSkademliaNodeIDs checks whether node IDs satisfy S/Kademlia cryptopuzzles
	EnforceSkademliaNodeIDs bool

	Routes *RoutingTable
}

var (
	PluginID                         = (*Plugin)(nil)
	_        network.PluginInterface = (*Plugin)(nil)
)

func (state *Plugin) Startup(net *network.Network) {
	// Create routing table.
	state.Routes = CreateRoutingTable(net.ID)
}

func (state *Plugin) Receive(ctx *network.PluginContext) error {
	sender := ctx.Sender()
	if state.EnforceSkademliaNodeIDs && !IsPeerValid(sender) {
		return errors.Errorf("Sender %v is not a valid node ID", sender)
	}
	// Update routing for every incoming message.
	state.Routes.Update(sender)
	gCtx := network.WithSignMessage(context.Background(), true)

	// Handle RPC.
	switch msg := ctx.Message().(type) {
	case *protobuf.Ping:
		if state.DisablePing {
			break
		}

		// Send pong to peer.
		err := ctx.Reply(gCtx, &protobuf.Pong{})

		if err != nil {
			return err
		}
	case *protobuf.Pong:
		if state.DisablePong {
			break
		}

		peers := FindNode(ctx.Network(), ctx.Sender(), BucketSize, 8)

		// Update routing table w/ closest peers to self.
		for _, peerID := range peers {
			state.Routes.Update(peerID)
		}

		log.Debug().
			Strs("peers", state.Routes.GetPeerAddresses()).
			Msg("bootstrapped w/ peer(s)")
	case *protobuf.LookupNodeRequest:
		if state.DisableLookup {
			break
		}

		// Prepare response.
		response := &protobuf.LookupNodeResponse{}

		// Respond back with closest peers to a provided target.
		for _, peerID := range state.Routes.FindClosestPeers(peer.ID(*msg.Target), BucketSize) {
			id := protobuf.ID(peerID)
			response.Peers = append(response.Peers, &id)
		}

		err := ctx.Reply(gCtx, response)
		if err != nil {
			return err
		}

		log.Debug().
			Strs("peers", state.Routes.GetPeerAddresses()).
			Msg("connected to peer(s)")
	}

	return nil
}

func (state *Plugin) Cleanup(net *network.Network) {
	// TODO: Save routing table?
}

func (state *Plugin) PeerDisconnect(client *network.PeerClient) {
	// Delete peer if in routing table.
	if client.ID != nil {
		if state.Routes.PeerExists(*client.ID) {
			state.Routes.RemovePeer(*client.ID)

			log.Debug().
				Str("address", client.Network.ID.Address).
				Str("peer_address", client.ID.Address).
				Msg("peer has disconnected")
		}
	}
}
