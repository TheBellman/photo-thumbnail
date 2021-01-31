package main

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

type mockS3 struct{}

func testFileReader(name string) io.ReadCloser {
	f, err := os.Open(name)
	if err != nil {
		log.Fatalf("Failed to open %s", name)
	}
	return f
}

func testFile(name string) *[]byte {
	data, err := ioutil.ReadAll(testFileReader(name))
	if err != nil {
		log.Fatalf("Failed to read %s", name)
	}
	return &data
}

func (f *mockS3) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return &s3.PutObjectOutput{}, nil
}

func (f *mockS3) GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
	if *input.Key == "key/good.jpeg" {
		return &s3.GetObjectOutput{
			ContentType: aws.String("image/jpeg"),
			Body:        testFileReader("./test.jpeg"),
		}, nil
	}

	if *input.Key == "key/bad.jpeg" {
		return &s3.GetObjectOutput{
			ContentType: aws.String("text/plain"),
			Body:        testFileReader("./test.jpeg"),
		}, nil
	}

	return nil, errors.New("unexpected test key provided")
}

func Test_validatePrefix(t *testing.T) {
	type args struct {
		photoPrefix string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "empty", args: args{photoPrefix: ""}, want: DefaultSrcPrefix},
		{name: "nonempty", args: args{photoPrefix: "folder"}, want: "folder/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatePrefix(tt.args.photoPrefix, DefaultSrcPrefix); got != tt.want {
				t.Errorf("validatePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateDestination(t *testing.T) {
	type args struct {
		dest string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "empty", args: args{dest: ""}, want: DefaultBucket},
		{name: "nnonempty", args: args{dest: "mybucket"}, want: "mybucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateDestination(tt.args.dest); got != tt.want {
				t.Errorf("extractName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validateRegion(t *testing.T) {
	type args struct {
		region string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "empty", args: args{region: ""}, want: DefaultRegion},
		{name: "nonempty", args: args{region: "us-east-1"}, want: "us-east-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateRegion(tt.args.region); got != tt.want {
				t.Errorf("validateRegion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getImageReader(t *testing.T) {
	mock := mockS3{}
	_, err := getImageReader(&mock, "bucket", "key/good.jpeg")
	if err != nil {
		t.Errorf("Received an unexpected error: %v", err)
	}

	_, err = getImageReader(&mock, "bucket", "key/bad.jpeg")
	if err == nil {
		t.Errorf("Did not get an error when expected")
	}
}

func Test_getImage(t *testing.T) {
	data, err := getImage(testFileReader("./test.jpeg"))
	if err != nil {
		t.Errorf("unexpected error loading file: %v", err)
	}
	if len(*data) == 0 {
		t.Errorf("empty byte slice returned!")
	}
}

func Test_resizeImage(t *testing.T) {
	img, err := resizeImage(testFile("./IMG_0348.jpeg"))
	if err != nil {
		t.Errorf("failed to resize image: %v", err)
	}

	if img == nil || len(*img) == 0 {
		t.Error("did not receive an image when it was expected")
	}
}

// This is not a good test. It will work locally because at least some
// credential can be resolved, and will work on CodeBuild because there
// will be something that resolves as well.
func Test_makeAWSSession(t *testing.T) {
	sess, err := makeAWSSession("eu-west-2")
	if err != nil {
		t.Errorf("failed to make a sesion: %v", err)
	}

	if sess == nil {
		t.Error("did not get an image, but no error was thrown")
	}
}

func Test_makeThumbKey(t *testing.T) {
	key := "photos/2020/12/23/fred"
	want := "thumbs/2020/12/23/fred"
	if got := makeThumbKey(key); got != want {
		t.Errorf("got: %q, want %q", got, want)
	}
}

func Test_saveThumbnail(t *testing.T) {
	mock := mockS3{}

	err := saveThumbnail(&mock, testFile("./IMG_0348.jpeg"), "bucket", "good")
	if err != nil {
		t.Errorf("unexpected error : %v", err)
	}
}