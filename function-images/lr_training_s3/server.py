from concurrent import futures
import logging

import grpc
from minio import Minio
from minio.error import (ResponseError, BucketAlreadyOwnedByYou,
                         BucketAlreadyExists)
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
import joblib
import pandas as pd
import re
import io

import helloworld_pb2
import helloworld_pb2_grpc

cleanup_re = re.compile('[^a-z]+')

responses = ["record_response", "replay_response"]

def cleanup(sentence):
    sentence = sentence.lower()
    sentence = cleanup_re.sub(' ', sentence).strip()
    return sentence

df_name = 'dataset.csv'
df2_name = 'dataset2.csv'
df_path = '/pulled_' + df_name
df2_path = 'pulled_' + df2_name


class Greeter(helloworld_pb2_grpc.GreeterServicer):

    def SayHello(self, request, context):
        minioClient = Minio('128.110.154.105:9000',
                access_key='minioadmin',
                secret_key='minioadmin',
                secure=False)

        if request.name == "record":
            msg = 'Hello, %s!' % responses[0]
            minioClient.fget_object('mybucket', df_name, df_path)

            df = pd.read_csv(df_path)
            df['train'] = df['Text'].apply(cleanup)
            tfidf_vector = TfidfVectorizer(min_df=100).fit(df['train'])
            train = tfidf_vector.transform(df['train'])
            model = LogisticRegression()
            model.fit(train, df['Score'])
        elif request.name == "replay":
            msg = 'Hello, %s!' % responses[1]
            minioClient.fget_object('mybucket', df2_name, df2_path)

            df2 = pd.read_csv(df2_path)
            df2['train'] = df2['Text'].apply(cleanup)
            tfidf_vector2 = TfidfVectorizer(min_df=100).fit(df2['train'])
            train2 = tfidf_vector2.transform(df2['train'])
            model2 = LogisticRegression()
            model2.fit(train2, df2['Score'])
        else:
            msg = 'Hello, %s!' % request.name
            minioClient.fget_object('mybucket', df_name, df_path)

            df = pd.read_csv(df_path)
            df['train'] = df['Text'].apply(cleanup)
            tfidf_vector = TfidfVectorizer(min_df=100).fit(df['train'])
            train = tfidf_vector.transform(df['train'])
            model = LogisticRegression()
            model.fit(train, df['Score'])

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
