package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

type NetworkEdge struct {
	Id            string `json:"id"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	Mainstat      uint64 `json:"mainstat,omitempty"`
	Secondarystat uint64 `json:"secondarystat,omitempty"`
}
type NetworkNode struct {
	Id       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Subtitle uint64 `json:"subtitle,omitempty"`
	Mainstat string `json:"mainstat,omitempty"`
	Color    string `json:"color"`
}
type NetworkGraph struct {
	Edges []NetworkEdge `json:"edges"`
	Nodes []NetworkNode `json:"nodes"`
}

var (
	defaultListenAddress = getEnv("LISTEN_ADDRESS", ":9114")
	defaultRpcAddr       = getEnv("RPC_ADDR", "localhost:10009")
	defaultTLSCertPath   = getEnv("TLS_CERT_PATH", "/root/.lnd")
	defaultMacaroonPath  = getEnv("MACAROON_PATH", "")
)
var (
	listenAddr = flag.String("web.listen-address", defaultListenAddress,
		"An address to listen on for web interface and telemetry. The default value can be overwritten by LISTEN_ADDRESS environment variable.")
	rpcAddr = flag.String("rpc.addr", defaultRpcAddr,
		"Lightning node RPC host. The default value can be overwritten by RPC_HOST environment variable.")
	tlsCertPath = flag.String("lnd.tls-cert-path", defaultTLSCertPath,
		"The path to the tls certificate. The default value can be overwritten by TLS_CERT_PATH environment variable.")
	macaroonPath = flag.String("lnd.macaroon-path", defaultMacaroonPath,
		"The path to the read only macaroon. The default value can be overwritten by MACAROON_PATH environment variable.")
)

func main() {
	flag.Parse()
	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{}) // return 200
	})
	http.HandleFunc("/api/graph/data", func(w http.ResponseWriter, r *http.Request) {
		graph, err := buildGraph(*rpcAddr, *tlsCertPath, *macaroonPath)
		if err != nil {
			log.Printf("error when getting graph: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Error"))
			return
		}
		bytes, _ := json.Marshal(graph)
		w.Write(bytes)
	})
	http.HandleFunc("/api/graph/fields", func(w http.ResponseWriter, r *http.Request) {
		type field struct {
			Field_name string `json:"field_name"`
			Field_type string `json:"type"`
		}
		type nodefield struct {
			Edges_fields []field `json:"edges_fields"`
			Nodes_fields []field `json:"nodes_fields"`
		}
		fieldStructure := nodefield{
			Edges_fields: []field{
				{
					Field_name: "id",
					Field_type: "string",
				},
				{
					Field_name: "source",
					Field_type: "string",
				},
				{
					Field_name: "target",
					Field_type: "string",
				},
				{
					Field_name: "mainstat",
					Field_type: "string",
				},
				{
					Field_name: "secondarystat",
					Field_type: "number",
				},
			},
			Nodes_fields: []field{
				{
					Field_name: "id",
					Field_type: "string",
				},
				{
					Field_name: "title",
					Field_type: "string",
				},
				{
					Field_name: "subtitle",
					Field_type: "number",
				},
				{
					Field_name: "mainstat",
					Field_type: "string",
				},
				{
					Field_name: "color",
					Field_type: "string",
				},
			},
		}
		bytes, _ := json.Marshal(fieldStructure)
		w.Write(bytes)
	})
	log.Printf("ListenAndServe %s \n", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func buildGraph(rpcAddr, tlsCertPath, macaroonPath string) (graph NetworkGraph, err error) {
	con, err := getGrpcClient(rpcAddr, tlsCertPath, macaroonPath)
	if err != nil {
		return NetworkGraph{}, err
	}
	rpcClient := lnrpc.NewLightningClient(con)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	parsedGraph := NetworkGraph{}
	graphNodeCache := map[string]string{}
	nodeInfo, err := rpcClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return NetworkGraph{}, err
	}
	if networkGraph, err := rpcClient.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{}); err == nil {
		for _, node := range networkGraph.Nodes {
			if node.LastUpdate <= 0 && node.Alias == "" && (node.Color == "" || node.Color == "#000000") {
				continue
			}
			graphNode := NetworkNode{
				Id:       node.PubKey,
				Title:    node.Alias,
				Subtitle: uint64(node.LastUpdate),
			}
			if node.Color != "" {
				graphNode.Color = node.Color
			}
			if nodeInfo.IdentityPubkey == node.PubKey {
				graphNode.Mainstat = "true"
			}
			parsedGraph.Nodes = append(parsedGraph.Nodes, graphNode)
			graphNodeCache[node.PubKey] = ""
		}

		for _, edge := range networkGraph.Edges {
			if isEdgeOrphaned(graphNodeCache, edge) {
				continue
			}
			graphEdge := NetworkEdge{
				Id:       strconv.FormatUint(edge.ChannelId, 10),
				Source:   edge.Node1Pub,
				Target:   edge.Node2Pub,
				Mainstat: uint64(edge.Capacity),
			}
			parsedGraph.Edges = append(parsedGraph.Edges, graphEdge)
		}
		return parsedGraph, nil
	} else {
		log.Printf("rpcClient.DescribeGraph err: %s", err)
		return parsedGraph, err
	}
}

func getGrpcClient(rpcAddr string, tlsCertPath string, macaroonPath string) (*grpc.ClientConn, error) {
	tlsCreds, err := credentials.NewClientTLSFromFile(tlsCertPath, "")
	if err != nil {
		log.Println("Cannot get node tls credentials", err)
		return nil, err
	}

	macaroonBytes, err := os.ReadFile(macaroonPath)
	if err != nil {
		log.Println("Cannot read macaroon file", err)
		return nil, err
	}

	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macaroonBytes); err != nil {
		log.Println("Cannot unmarshal macaroon", err)
		return nil, err
	}

	macOpts, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(macOpts),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1 * 1024 * 1024 * 50)),
	}

	log.Printf("dialing rpcAddr: %s", rpcAddr)
	conn, err := grpc.Dial(rpcAddr, opts...)
	if err != nil {
		log.Printf("grpc.Dial() err: %s", err)
		return nil, err
	}

	return conn, nil
}

func getEnv(key, defaultValue string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}
	return value
}

func isEdgeOrphaned(graphNodeCache map[string]string, edge *lnrpc.ChannelEdge) bool {
	_, node1Ok := graphNodeCache[edge.Node1Pub]
	_, node2Ok := graphNodeCache[edge.Node2Pub]

	if node1Ok && node2Ok {
		return false
	}
	return true
}
