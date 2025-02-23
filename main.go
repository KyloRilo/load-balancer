package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

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
	grpcPort              = 50501
	port                  = 3030
)

type Server struct {
	proto.UnimplementedLoadBalancerServer
}

func (s *Server) AddConn(_ context.Context, in *proto.AddConnReq) (*proto.AddConnResp, error) {
	log.Printf("Received: %v", in.String())
	err := Balancer.AddConnection(in.GetUrl())
	if err != nil {
		return &proto.AddConnResp{
			Code: 500,
		}, err
	}

	return &proto.AddConnResp{
		Code: 200,
	}, nil
}

func NewServer() proto.LoadBalancerServer {
	return &Server{}
}

func serveGRPC() {
	log.Print("Serving GRPC...")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	proto.RegisterLoadBalancerServer(s, NewServer())
	reflection.Register(s)

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func serveHTTP() {
	log.Print("Serving HTTP")
	httpServer := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(Balancer.balance),
	}

	log.Printf("Load Balancer started at :%d\n", port)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	log.Printf("Init...")
	go serveGRPC()
	go serveHTTP()
	go Balancer.healthCheck()
	time.Sleep(time.Hour)
}
