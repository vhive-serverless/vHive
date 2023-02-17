package main

import (
	"context"
	//"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	minioEndpoint := "10.96.0.46:9000"
	minioAccessKey := "minio"
	minioSceretKey := "minio123"
	minioBucket := "mybucket"

	log.Debug("Creating minio client")
	// Initialize minio client object.
	minioClient, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSceretKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalln(err)
	}

	log.Debug("Checking if bucket exists")
	err = minioClient.MakeBucket(ctx, minioBucket, minio.MakeBucketOptions{})
	if err != nil {
		// Check to see if bucket exists	<-- we assume we create the bucket
		// in the script so we dont  need to take care of this in the code
		exists, errBucketExists := minioClient.BucketExists(ctx, minioBucket)
		if errBucketExists == nil && exists {
			log.Printf("We already own %s\n", minioBucket)
		} else {
			log.Fatalln(err)
		}
	} else {
		log.Printf("Successfully created %s\n", minioBucket)
	}

	functionName := "rnn"
	objectName := "patchfile" + functionName
	//objectName := "testFile"

	filePath := "/users/estellan/vhive/rnn/patchfile"
	//filePath := "/users/estellan/vhive/testFile"

	contentType := "application/octet-stream"

	infoPut, errPut := minioClient.FPutObject(ctx, minioBucket, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
	if errPut != nil {
		log.Fatalln(errPut)
	}

	log.Printf("Successfully uploaded %s of size %d\n", objectName, infoPut.Size)

	objectName = "infofile" + functionName
	//objectName = "testFile"

	filePath = "/users/estellan/vhive/rnn/infofile"
	//filePath = "/users/estellan/vhive/testFile"

	contentType = "application/octet-stream"

	infoPut, errPut = minioClient.FPutObject(ctx, minioBucket, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
	if errPut != nil {
		log.Fatalln(errPut)
	}

	log.Printf("Successfully uploaded %s of size %d\n", objectName, infoPut.Size)

	objectName = "snapfile" + functionName
	//objectName = "testFile"

	filePath = "/users/estellan/vhive/rnn/snapfile"
	//filePath = "/users/estellan/vhive/testFile"

	contentType = "application/octet-stream"

	infoPut, errPut = minioClient.FPutObject(ctx, minioBucket, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
	if errPut != nil {
		log.Fatalln(errPut)
	}

	log.Printf("Successfully uploaded %s of size %d\n", objectName, infoPut.Size)

	objectName = "memfile" + functionName
	//objectName = "testFile"

	filePath = "/users/estellan/vhive/rnn/memfile"
	//filePath = "/users/estellan/vhive/testFile"

	contentType = "application/octet-stream"

	infoPut, errPut = minioClient.FPutObject(ctx, minioBucket, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
	if errPut != nil {
		log.Fatalln(errPut)
	}

	log.Printf("Successfully uploaded %s of size %d\n", objectName, infoPut.Size)

	//log.Printf("Random prints to take time\n")
	//time.Sleep(1000 * time.Second)
	//
	//
	//errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath,  minio.GetObjectOptions{})
	//if errGet != nil {
	//	log.Fatalln(errGet)
	//}

	//log.Printf("Successfully downloaded the patch file %d\n", objectName, infoGet.Size)
}
