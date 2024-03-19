package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/PedrobyJoao/koko/k_protocol"
	"github.com/PedrobyJoao/koko/libp2p"
	"github.com/PedrobyJoao/koko/utils"
)

func getPeersFromPeerStore(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(libp2p.GetPeersFromPeerStore())
}

func getIndexOfNodes(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(k_protocol.IndexOfCPs)
}

func handleErrorResponse(w http.ResponseWriter, err error) {
	log.Printf("%v", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func Serve() {
	router := mux.NewRouter()
	router.HandleFunc("/peer_store/peers", getPeersFromPeerStore).Methods("GET")
	router.HandleFunc("/cpsIndex", getIndexOfNodes).Methods("GET")

	port, err := utils.FindFreePort()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("API Listening on port %d", port)

	log.Fatal(http.ListenAndServe(":"+fmt.Sprint(port), router))
}
