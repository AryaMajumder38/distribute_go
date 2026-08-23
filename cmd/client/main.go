package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	api "github.com/travisjeffery/proglog/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", ":8400", "service address")
	produce := flag.String("produce", "", "message to append to the log")
	consume := flag.Uint64("consume", 0, "offset to read")
	flag.Parse()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	client := api.NewLogClient(conn)
	ctx := context.Background()

	if *produce != "" {
		res, err := client.Produce(ctx, &api.ProduceRequest{
			Record: &api.Record{Value: []byte(*produce)},
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("produced %q -> offset %d\n", *produce, res.Offset)
		return
	}

	res, err := client.Consume(ctx, &api.ConsumeRequest{Offset: *consume})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("consumed offset %d: %q\n", res.Record.Offset, res.Record.Value)
}
