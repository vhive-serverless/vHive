package main

import (
	"context"
	"fmt"
	"time"

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
	// Check to see if bucket exists	<-- we assume we create the bucket
	// in the script so we dont  need to take care of this in the code
	exists, errBucketExists := minioClient.BucketExists(ctx, minioBucket)
	if errBucketExists == nil && exists {
		log.Printf("Bucket exists %s\n", minioBucket)
	} else {
		log.Fatalln(err)
	}

	functionName := "hello"
	objectName := "patchfile" + functionName
	//objectName := "testFile"
	//filePath := "/users/estellan/vhive/testFile"
	filePath := "/users/estellan/vhive/patchfile"

	tStartCold := time.Now()
	errGet := minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs := time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("patchfilehello %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "infofile" + functionName
	//objectName := "testFile"
	//filePath := "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/infofile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("infofilehello %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "snapfile" + functionName
	//objectName := "testFile"
	//filePath := "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/snapfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("snapfilehello %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "memfile" + functionName
	//objectName := "testFile"
	//filePath := "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/memfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("memfilehello %d\n", coldStartTimeMs)
	fmt.Print("\n")

	functionName = "pyaes"
	objectName = "patchfile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/patchfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("patchfilepyaes %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "infofile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/infofile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("infofilepyaes %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "snapfile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/snapfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("snapfilepyaes %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "memfile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/memfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("memfilepyaes %d\n", coldStartTimeMs)
	fmt.Print("\n")

	functionName = "rnn"
	objectName = "patchfile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/patchfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("patchfilernn %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "infofile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/infofile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("infofilernn %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "snapfile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/snapfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("snapfilernn %d\n", coldStartTimeMs)
	fmt.Print("\n")

	objectName = "memfile" + functionName
	//objectName = "testFile"
	//filePath = "/users/estellan/vhive/testFile"
	filePath = "/users/estellan/vhive/memfile"

	tStartCold = time.Now()
	errGet = minioClient.FGetObject(ctx, minioBucket, objectName, filePath, minio.GetObjectOptions{})
	coldStartTimeMs = time.Since(tStartCold)
	if errGet != nil {
		log.Fatalln(errGet)
	}

	fmt.Print("memfilernn %d\n", coldStartTimeMs)
	fmt.Print("\n")
	// log.Printf("Successfully downloaded the patch file %d\n", objectName)

	// objectCh := minioClient.ListObjects(ctx, minioBucket, minio.ListObjectsOptions{
	// 	Prefix: "patch",
	// 	Recursive: true,
	// })
	// for object := range objectCh {
	// 	if object.Err != nil {
	// 		fmt.Println(object.Err)
	// 		return
	// 	}
	// 	fmt.Println(object.Key)
	// 	fmt.Println(object.UserMetadata)
	// 	fmt.Println(object.UserTags)
	// 	fmt.Println(object.UserTagCount)
	// 	fmt.Println(object.Owner)

	// }
}
