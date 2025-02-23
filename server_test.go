package main

import (
	"context"
	"log"
	"net"
	"testing"

	"github.com/KyloRilo/load-balancer/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type testCase struct {
	name string
	in   interface{}
	out  interface{}
	err  error
}

func server() (proto.LoadBalancerClient, func()) {
	buffer := 101024 * 1024
	lis := bufconn.Listen(buffer)
	baseServer := grpc.NewServer()
	proto.RegisterLoadBalancerServer(baseServer, NewServer())

	log.Printf("server listening at %v", lis.Addr())
	go func() {
		if err := baseServer.Serve(lis); err != nil {
			log.Printf("failed to serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient("", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		log.Printf("failed connecting to server")
	}

	go Balancer.healthCheck()
	log.Print("Test server running...")

	client := proto.NewLoadBalancerClient(conn)
	return client, func() {
		err := lis.Close()
		if err != nil {
			log.Printf("error, closing listener: %v", err)
		}
		baseServer.Stop()
	}
}

func testValidator(t *testing.T, test testCase, out interface{}, err error) {
	log.Println("Validating test case: ", test)
	if err != nil && test.err != err {
		t.Errorf("Detected Error mismatch => Wanted: %v, Received: %v", test.err, err)
	}

	if out != nil && test.out != out {
		t.Errorf("Detected Resp mismatch => Wanted: %v, Received: %v", test.out, out)
	}
}

func TestLbServer_AddConn(t *testing.T) {
	log.Print("Init AddConn tests")
	ctx := context.Background()
	client, closer := server()
	defer closer()

	tests := []testCase{
		{
			name: "AddConnection_Succeeds",
			in: &proto.AddConnReq{
				Url: "google.com",
			},
			out: &proto.AddConnResp{},
		},
	}

	log.Println("Init test cases: ", tests)
	for _, test := range tests {
		resp, err := client.AddConn(ctx, test.in.(*proto.AddConnReq))
		testValidator(t, test, resp, err)
	}
}
