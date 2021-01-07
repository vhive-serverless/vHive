from concurrent import futures
import logging

import grpc

import helloworld_pb2
import helloworld_pb2_grpc

import tensorflow as tf
from tensorflow.python.keras.preprocessing import image
from tensorflow.python.keras.applications.resnet50 import preprocess_input, decode_predictions
import numpy as np

from squeezenet import SqueezeNet

responses = ["record_response", "replay_response"]

session_conf = tf.ConfigProto(
              intra_op_parallelism_threads=1,
              inter_op_parallelism_threads=1)
sess = tf.Session(config=session_conf)

img = image.load_img('/image.jpg', target_size=(227, 227))
model = SqueezeNet(weights='imagenet')
model._make_predict_function() 
print('Model is ready')

img2 = image.load_img('/image2.jpg', target_size=(227, 227))
model2 = SqueezeNet(weights='imagenet')
model2._make_predict_function() 
print('Model2 is ready')

class Greeter(helloworld_pb2_grpc.GreeterServicer):

    def SayHello(self, request, context):
        #res = decode_predictions(preds) # requires access to the Internet
        if request.name == "record":
            msg = 'Hello, %s!' % responses[0]
            x = image.img_to_array(img)
            x = np.expand_dims(x, axis=0)
            x = preprocess_input(x)
            preds = model.predict(x)
        elif request.name == "replay":
            msg = 'Hello, %s!' % responses[1]
            x2 = image.img_to_array(img2)
            x2 = np.expand_dims(x2, axis=0)
            x2 = preprocess_input(x2)
            preds2 = model.predict(x2)
        else:
            msg = 'Hello, %s!' % request.name
            x = image.img_to_array(img)
            x = np.expand_dims(x, axis=0)
            x = preprocess_input(x)
            preds = model.predict(x)

        #joblib.dump(model, '/var/local/dir/lr_model.pk')
        return helloworld_pb2.HelloReply(message=msg)


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
    helloworld_pb2_grpc.add_GreeterServicer_to_server(Greeter(), server)
    server.add_insecure_port('[::]:50051')
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    logging.basicConfig()
    serve()
