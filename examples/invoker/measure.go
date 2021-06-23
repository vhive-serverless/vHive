package main

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"eventing/proto"
)

func Start(tdbAddr string) (End func() time.Duration) {
	var dialOption grpc.DialOption
	if *withTracing {
		dialOption = grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())
	} else {
		dialOption = grpc.WithBlock()
	}
	conn, err := grpc.Dial(tdbAddr, grpc.WithInsecure(), dialOption)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	client := proto.NewTimeseriesClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := client.StartExperiment(ctx, &proto.ExperimentDefinition{
		CompletionEventDescriptors: []*proto.CompletionEventDescriptor{
			{
				AttrMatchers: map[string]string{
					// TODO: this varies from workload to workload
					//       a simple urls.txt is no longer sufficient, more metadata is needed
					"type": "greeting",
					"source": "consumer",
				},
			},
		},
	}); err != nil {
		log.Fatalln("failed to start experiment", err)
	}

	return func() time.Duration {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := client.EndExperiment(ctx, &empty.Empty{})
		if err != nil {
			log.Fatalln("failed to end experiment", err)
		}
		if len(res.Invocations) != 1 {
			log.Fatalf("wrong number of invocations: 1 != %d\n", len(res.Invocations))
		}
		if err := conn.Close(); err != nil {
			log.Warnln("failed to close tdb connection", err)
		}
		return res.Invocations[0].Duration.AsDuration()
	}
}
