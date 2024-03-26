package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/PedrobyJoao/koko/api"
	"github.com/PedrobyJoao/koko/db"
	"github.com/PedrobyJoao/koko/libp2p"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	port      = flag.Int("port", 0, "port number to listen on libp2p host")
	bootstrap = flag.Bool("bootstrap", true, "connect to bootstrap nodes")
	cp        = flag.Bool("cp", false, "act as compute providers")
)

func main() {
	flag.Parse()

	// Load the .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Retrieve the BOOTSTRAP_IP environment variable
	serverIP := os.Getenv("BOOTSTRAP_IP")
	if serverIP == "" {
		log.Fatal("BOOTSTRAP_IP is not set in the .env file")
	}

	// serverIP2 := os.Getenv("BOOTSTRAP_IP_2")
	// if serverIP2 == "" {
	// 	log.Fatal("BOOTSTRAP_IP_2 is not set in the .env file")
	// }

	var bootstrapPeers []string
	if *bootstrap {
		log.Println("Selected to connect to bootstrap nodes...")
		bootstrapPeers = []string{
			// Change it for your own bootstrap node
			fmt.Sprintf("/ip4/%s/tcp/8080/p2p/QmTgGSGFksvZgCkKu4sy621Qs9RpwnPdXFN9ZwB8ocViyF", serverIP),
			// fmt.Sprintf("/ip4/%s/tcp/7778/p2p/QmeHeb8SoTCXRbfeC1LZ1bB1VHSxRJSpJc5KuwnkuDr4Lv", serverIP2),
		}
	}

	log.Println("Initializing database...")
	err = db.New()
	if err != nil {
		log.Fatal(err)
	}

	host, err := libp2p.NewHost(*port, bootstrapPeers, *cp)
	if err != nil {
		log.Fatal(err)
	}

	// start a web server to expose metrics
	// serveMetricsToPrometheus()

	go api.Serve()

	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")

	// shut the node down
	if err := host.Close(); err != nil {
		log.Fatal(err)
	}
}

func serveMetricsToPrometheus() {
	go func() {
		http.Handle("/debug/metrics/prometheus", promhttp.Handler())
		log.Fatal(http.ListenAndServe("localhost:0", nil))
	}()
}
