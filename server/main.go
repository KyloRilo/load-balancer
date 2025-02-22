package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/KyloRilo/load-balancer/proto"
	"google.golang.org/grpc"
)

const (
	Attempts int = iota
	Retry
)

var (
	lb   LoadBalancer = *NewLoadBalancer()
	port              = flag.Int("port", 3030, "LoadBalancer port")
)

type server struct {
	proto.UnimplementedLoadBalancerServer
}

func (s *server) AddConnection(_ context.Context, in *proto.ConnRequest) (*proto.ConnResp, error) {
	log.Printf("Received: %v", in.String())
	err := lb.AddConnection(in.GetUrl())
	if err != nil {
		return &proto.ConnResp{
			Code: 500,
		}, err
	}

	return &proto.ConnResp{
		Code: 200,
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	proto.RegisterLoadBalancerServer(s, &server{})

	go lb.healthCheck()

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
