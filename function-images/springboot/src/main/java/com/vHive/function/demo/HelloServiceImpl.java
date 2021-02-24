package com.vHive.function.demo;

import io.grpc.examples.helloworld.GreeterGrpc;
import io.grpc.examples.helloworld.HelloRequest;
import io.grpc.examples.helloworld.HelloReply;

import io.grpc.stub.StreamObserver;

import net.devh.boot.grpc.server.service.GrpcService;

@GrpcService
public class HelloServiceImpl extends GreeterGrpc.GreeterImplBase {
    static String[] responses = new String[]{"record_response", "replay_response"};

    @Override
    public void sayHello(HelloRequest request, StreamObserver<HelloReply> responseObserver) {
        String replyMessage = String.format("Hello, %s!", request.getName());
        
        if(request.getName().equals("record"))
            replyMessage = String.format("Hello, %s!", responses[0]);
        else if(request.getName().equals("replay"))
            replyMessage = String.format("Hello, %s!", responses[1]);
    
        HelloReply reply = HelloReply.newBuilder()
                .setMessage(replyMessage)
                .build();
        responseObserver.onNext(reply);
        responseObserver.onCompleted();
    }

}
