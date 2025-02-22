package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/KyloRilo/load-balancer/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	Attempts int = iota
	Retry
)

var (
	Balancer LoadBalancer = *NewLoadBalancer()
	Port                  = flag.Int("port", 3030, "LoadBalancer port")
)

type Server struct {
	proto.UnimplementedLoadBalancerServer
}

func (s *Server) AddConnection(_ context.Context, in *proto.ConnRequest) (*proto.ConnResp, error) {
	log.Printf("Received: %v", in.String())
	err := Balancer.AddConnection(in.GetUrl())
	if err != nil {
		return &proto.ConnResp{
			Code: 500,
		}, err
	}

	return &proto.ConnResp{
		Code: 200,
	}, nil
}

func NewServer() proto.LoadBalancerServer {
	return &Server{}
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	proto.RegisterLoadBalancerServer(s, NewServer())
	reflection.Register(s)

	go Balancer.healthCheck()
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
