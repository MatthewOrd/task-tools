package s3

import (
	"bytes"
	"fmt"
	"log"

	"github.com/minio/minio-go"
)

func createTestFile(pth string) error {
	w, err := NewWriter(pth, testAccessKey, testSecretKey, nil)
	if err != nil {
		return err
	}
	w.WriteLine([]byte("test line"))
	w.WriteLine([]byte("test line"))
	err = w.Close()
	return err
}

func rmTestFile(pth string) error {
	bckt, objPth := parsePth(pth)
	return testS3Client.RemoveObject(bckt, objPth)
}

func ExampleNewReader() {
	pth := fmt.Sprintf("s3://%v/read/test.txt", testBucket)
	r, err := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	fmt.Println(err)        // output: <nil>
	fmt.Println(r.sts.Path) // output: s3://task-tools-test/read/test.txt
	fmt.Println(r.sts.Size) // output: 20

	// Output:
	// <nil>
	// s3://task-tools-test/read/test.txt
	// 20
}

func ExampleNewReaderErrBadClient() {
	origHost := StoreHost
	StoreHost = "bad/endpoint/"
	r, err := NewReader("", "", "")
	if err == nil {
		return
	}

	fmt.Println(r)   // output: <nil>
	fmt.Println(err) // output: Endpoint: bad/endpoint/ does not follow ip address or domain name standards.

	// restore endpoint
	StoreHost = origHost

	// Output:
	// <nil>
	// Endpoint: bad/endpoint/ does not follow ip address or domain name standards.
}

func ExampleNewReaderErrBadObject() {
	r, err := NewReader("", testAccessKey, testSecretKey)

	fmt.Println(r)   // output: <nil>
	fmt.Println(err) // output: Bucket name cannot be empty

	// Output:
	// <nil>
	// Bucket name cannot be empty
}

func ExampleNewReaderErrObjStat() {
	pth := "s3://does-not/exist.txt"
	r, err := NewReader(pth, "", "")

	fmt.Println(r)   // output: <nil>
	fmt.Println(err) // output: The specified bucket does not exist.

	// Output:
	// <nil>
	// The specified bucket does not exist
}

func ExampleNewReaderErrGzip() {
	// write a normal file to s3 as if it
	// were a gzip file. NewReader will see
	// the .gz extension and read it as a
	// gz file. Since it's not and there is
	// no gz header, it will return and error.

	// create 'bad' gz file.
	var buf bytes.Buffer
	buf.Write([]byte("test line\n"))
	buf.Write([]byte("test line\n"))
	opts := minio.PutObjectOptions{}
	opts.ContentType = "application/octet-stream"
	_, err := testS3Client.PutObject(
		testBucket,
		"bad.gz",
		&buf,
		20,
		opts,
	)
	if err != nil {
		log.Println(err)
		return
	}

	pth := fmt.Sprintf("s3://%v/bad.gz", testBucket)
	r, err := NewReader(pth, testAccessKey, testSecretKey)

	fmt.Println(r)   // output: <nil>
	fmt.Println(err) // output: gzip: invalid header

	// cleanup file
	rmTestFile(pth)

	// Output:
	// <nil>
	// gzip: invalid header
}

func ExampleReader_Read() {
	pth := fmt.Sprintf("s3://%v/read/test.txt", testBucket)
	r, err := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	b := make([]byte, 20)
	n, err := r.Read(b)

	fmt.Println(n)             // output: 20
	fmt.Println(err)           // output: <nil>
	fmt.Print(string(b))       // output: test line, test line
	fmt.Println(r.sts.ByteCnt) // output: 20
	fmt.Println(r.sts.LineCnt) // output: 0

	// Output:
	// 20
	// <nil>
	// test line
	// test line
	// 20
	// 0
}

func ExampleReader_ReadCompressed() {
	pth := fmt.Sprintf("s3://%v/read/test.gz", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	b := make([]byte, 20)
	n, err := r.Read(b)

	fmt.Println(n)             // output: 20
	fmt.Println(err)           // output: <nil>
	fmt.Print(string(b))       // output: test line, test line
	fmt.Println(r.sts.ByteCnt) // output: 20
	fmt.Println(r.sts.LineCnt) // output: 0

	// Output:
	// 20
	// <nil>
	// test line
	// test line
	// 20
	// 0
}

func ExampleReader_ReadLine() {
	pth := fmt.Sprintf("s3://%v/read/test.txt", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	ln1, err1 := r.ReadLine()
	ln2, err2 := r.ReadLine()

	fmt.Println(string(ln1))   // output: test line
	fmt.Println(err1)          // output: <nil>
	fmt.Println(string(ln2))   // output: test line
	fmt.Println(err2)          // output: <nil>
	fmt.Println(r.sts.ByteCnt) // output: 20
	fmt.Println(r.sts.LineCnt) // output: 2

	// Output:
	// test line
	// <nil>
	// test line
	// <nil>
	// 20
	// 2
}

func ExampleReader_ReadLineCompressed() {
	pth := fmt.Sprintf("s3://%v/read/test.gz", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	ln1, err1 := r.ReadLine()
	ln2, err2 := r.ReadLine()
	ln3, err3 := r.ReadLine() // EOF

	fmt.Println(string(ln1))   // output: test line
	fmt.Println(err1)          // output: <nil>
	fmt.Println(string(ln2))   // output: test line
	fmt.Println(err2)          // output: <nil>
	fmt.Println(string(ln3))   // output:
	fmt.Println(err3)          // output: EOF
	fmt.Println(r.sts.ByteCnt) // output: 20
	fmt.Println(r.sts.LineCnt) // output: 2

	// Output:
	// test line
	// <nil>
	// test line
	// <nil>
	//
	// EOF
	// 20
	// 2
}

func ExampleReader_Stats() {
	pth := fmt.Sprintf("s3://%v/read/test.txt", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	r.ReadLine()
	sts := r.Stats()
	fmt.Println(sts.ByteCnt) // output: 10
	fmt.Println(sts.LineCnt) // output: 1

	// Output:
	// 10
	// 1
}

func ExampleReader_Close() {
	pth := fmt.Sprintf("s3://%v/read/test.txt", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	r.ReadLine()
	r.ReadLine()
	r.ReadLine()
	err := r.Close()
	sts := r.Stats()

	fmt.Println(err)          // output: <nil>
	fmt.Println(sts.ByteCnt)  // output: 20
	fmt.Println(sts.LineCnt)  // output: 2
	fmt.Println(sts.Checksum) // output: 54f30d75cf7374c7e524a4530dbc93c2

	// Output:
	// <nil>
	// 20
	// 2
	// 54f30d75cf7374c7e524a4530dbc93c2
}

func ExampleReader_CloseCompressed() {
	pth := fmt.Sprintf("s3://%v/read/test.gz", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	r.ReadLine()
	r.ReadLine()
	r.ReadLine()
	err := r.Close()
	sts := r.Stats()

	fmt.Println(err)          // output: <nil>
	fmt.Println(sts.ByteCnt)  // output: 20
	fmt.Println(sts.LineCnt)  // output: 2
	fmt.Println(sts.Checksum) // output: 42e649f9834028184ec21940d13a300f

	// Output:
	// <nil>
	// 20
	// 2
	// 42e649f9834028184ec21940d13a300f
}

func ExampleReader_CloseandClose() {
	pth := fmt.Sprintf("s3://%v/read/test.gz", testBucket)
	r, _ := NewReader(pth, testAccessKey, testSecretKey)
	if r == nil {
		return
	}

	r.ReadLine()
	r.ReadLine()
	r.ReadLine()
	err1 := r.Close()
	err2 := r.Close()
	sts := r.Stats()

	fmt.Println(err1)         // output: <nil>
	fmt.Println(err2)         // output: <nil>
	fmt.Println(sts.ByteCnt)  // output: 20
	fmt.Println(sts.LineCnt)  // output: 2
	fmt.Println(sts.Checksum) // output: 42e649f9834028184ec21940d13a300f

	// Output:
	// <nil>
	// <nil>
	// 20
	// 2
	// 42e649f9834028184ec21940d13a300f
}