package cli

import (
	"context"
	"testing"

	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/boxo/routing/http/client"
	"github.com/ipfs/boxo/routing/http/types"
	"github.com/ipfs/boxo/routing/http/types/iter"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestRoutingV1(t *testing.T) {
	t.Parallel()

	nodes := harness.NewT(t).NewNodes(5).Init()
	nodes.ForEachPar(func(node *harness.Node) {
		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.ExposeRoutingAPI = config.True
			cfg.Routing.Type = config.NewOptionalString("dht")
		})
	})
	nodes.StartDaemons().Connect()

	t.Run("Get Providers Responds With Correct Peers", func(t *testing.T) {
		t.Parallel()

		cidStr := nodes[2].IPFSAddStr("hello world")
		_ = nodes[3].IPFSAddStr("hello world")

		cid, err := cid.Decode(cidStr)
		assert.NoError(t, err)

		c, err := client.New(nodes[1].GatewayURL())
		assert.NoError(t, err)

		resultsIter, err := c.GetProviders(context.Background(), cid)
		assert.NoError(t, err)

		records, err := iter.ReadAllResults(resultsIter)
		assert.NoError(t, err)

		var peers []peer.ID
		for _, record := range records {
			assert.Equal(t, types.SchemaPeer, record.GetSchema())

			peer, ok := record.(*types.PeerRecord)
			assert.True(t, ok)
			peers = append(peers, *peer.ID)
		}

		assert.Contains(t, peers, nodes[2].PeerID())
		assert.Contains(t, peers, nodes[3].PeerID())
	})

	t.Run("Get Peers Responds With Correct Peers", func(t *testing.T) {
		t.Parallel()

		c, err := client.New(nodes[1].GatewayURL())
		assert.NoError(t, err)

		resultsIter, err := c.GetPeers(context.Background(), nodes[2].PeerID())
		assert.NoError(t, err)

		records, err := iter.ReadAllResults(resultsIter)
		assert.NoError(t, err)
		assert.Len(t, records, 1)
		assert.IsType(t, records[0].GetSchema(), records[0].GetSchema())
		assert.IsType(t, records[0], &types.PeerRecord{})

		peer := records[0].(*types.PeerRecord)
		assert.Equal(t, nodes[2].PeerID().String(), peer.ID.String())
		assert.NotEmpty(t, peer.Addrs)
	})

	t.Run("Get IPNS Record Responds With Correct Record", func(t *testing.T) {
		t.Parallel()

		cidStr := nodes[0].IPFSAddStr("hello get ipns test")
		nodes[0].IPFS("name", "publish", "--allow-offline", cidStr)

		// Ask for record from a different peer.
		c, err := client.New(nodes[1].GatewayURL())
		assert.NoError(t, err)

		record, err := c.GetIPNSRecord(context.Background(), ipns.NameFromPeer(nodes[0].PeerID()))
		assert.NoError(t, err)

		value, err := record.Value()
		assert.NoError(t, err)
		assert.Equal(t, "/ipfs/"+cidStr, value.String())
	})

	t.Run("Put IPNS Record Succeeds", func(t *testing.T) {
		t.Parallel()

		// Publish a record.
		cidStr := nodes[0].IPFSAddStr("hello get ipns test 2")
		nodes[0].IPFS("name", "publish", "--allow-offline", cidStr)
		c, err := client.New(nodes[0].GatewayURL())
		assert.NoError(t, err)
		record, err := c.GetIPNSRecord(context.Background(), ipns.NameFromPeer(nodes[0].PeerID()))
		assert.NoError(t, err)
		value, err := record.Value()
		assert.NoError(t, err)
		assert.Equal(t, "/ipfs/"+cidStr, value.String())

		// Start lonely node that is not connected to other nodes.
		node := harness.NewT(t).NewNode().Init()
		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.ExposeRoutingAPI = config.True
			cfg.Routing.Type = config.NewOptionalString("dht")
		})
		node.StartDaemon()

		// Put IPNS record in lonely node. It should be accepted as it is a valid record.
		c, err = client.New(node.GatewayURL())
		assert.NoError(t, err)
		err = c.PutIPNSRecord(context.Background(), ipns.NameFromPeer(nodes[0].PeerID()), record)
		assert.NoError(t, err)

		// Get the record from lonely node and double check.
		record, err = c.GetIPNSRecord(context.Background(), ipns.NameFromPeer(nodes[0].PeerID()))
		assert.NoError(t, err)
		value, err = record.Value()
		assert.NoError(t, err)
		assert.Equal(t, "/ipfs/"+cidStr, value.String())
	})
}
