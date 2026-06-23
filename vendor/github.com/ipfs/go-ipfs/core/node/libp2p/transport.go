package libp2p

import (
	config "github.com/ipfs/go-ipfs-config"
	libp2p "github.com/libp2p/go-libp2p"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	tcp "github.com/libp2p/go-tcp-transport"
	websocket "github.com/libp2p/go-ws-transport"

	"go.uber.org/fx"
)

func Transports(tptConfig config.Transports) interface{} {
	return func(pnet struct {
		fx.In
		Fprint PNetFingerprint `optional:"true"`
	}) (opts Libp2pOpts, err error) {
		if tptConfig.Network.TCP.WithDefault(true) {
			opts.Opts = append(opts.Opts, libp2p.Transport(tcp.NewTCPTransport))
		}

		if tptConfig.Network.Websocket.WithDefault(true) {
			opts.Opts = append(opts.Opts, libp2p.Transport(websocket.New))
		}

		return opts, nil
	}
}

func BandwidthCounter() (opts Libp2pOpts, reporter *metrics.BandwidthCounter) {
	reporter = metrics.NewBandwidthCounter()
	opts.Opts = append(opts.Opts, libp2p.BandwidthReporter(reporter))
	return opts, reporter
}
