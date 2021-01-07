from concurrent import futures
import logging

import grpc

from sklearn.feature_extraction.text import TfidfVectorizer
#from sklearn.externals import joblib
import joblib

import pandas as pd
import os
import re

import helloworld_pb2
import helloworld_pb2_grpc

cleanup_re = re.compile('[^a-z]+')

def cleanup(sentence):
    sentence = sentence.lower()
    sentence = cleanup_re.sub(' ', sentence).strip()
    return sentence

dataset = pd.read_csv('/dataset.csv')
#dataset = pd.read_csv('/var/local/dir/dataset.csv')
df_input = pd.DataFrame()
dataset['train'] = dataset['Text'].apply(cleanup)
tfidf_vect = TfidfVectorizer(min_df=100).fit(dataset['train'])
x = 'The ambiance is magical. The food and service was nice! The lobster and cheese was to die for and our steaks were cooked perfectly.  '
df_input['x'] = [x]
df_input['x'] = df_input['x'].apply(cleanup)
X = tfidf_vect.transform(df_input['x'])

x = 'My favorite cafe. I like going there on weekends, always taking a cafe and some of their pastry before visiting my parents.  '
df_input['x'] = [x]
df_input['x'] = df_input['x'].apply(cleanup)
X2 = tfidf_vect.transform(df_input['x'])

#model = joblib.load('/var/local/dir/lr_model.pk')
model = joblib.load('/lr_model.pk')
print('Model is ready')

responses = ["record_response", "replay_response"]

class Greeter(helloworld_pb2_grpc.GreeterServicer):

    def SayHello(self, request, context):
        if request.name == "record":
            msg = 'Hello, %s!' % responses[0]
            y = model.predict(X)
        elif request.name == "replay":
            msg = 'Hello, %s!' % responses[1]
            y = model.predict(X2)
        else:
            msg = 'Hello, %s!' % request.name
            y = model.predict(X)

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
