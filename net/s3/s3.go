package s3

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/masahide/goansible"
)

type S3 struct {
	Bucket      string `goansible:"bucket,required"`
	Region      string `goansible:"region"`
	PutFile     string `goansible:"put_file"`
	GetFile     string `goansible:"get_file"`
	Mkdir       bool   `goansible:"mkdir"`
	At          string `goansible:"at"`
	Public      bool   `goansible:"public"`
	ContentType string `goansible:"content_type"`
	Writable    bool   `goansible:"writable"`
	GZip        bool   `goansible:"gzip"`
}

type ioReaderCloser struct {
	io.Reader
}

func (i ioReaderCloser) Close() error {
	return nil //TODO: とりあえずダミークローザー 多分問題ないはず
}

func (s *S3) Run(env *goansible.CommandEnv) (*goansible.Result, error) {
	return s.s3cp(env, "", "")
}

func (s *S3) s3cp(env *goansible.CommandEnv, a, b string) (*goansible.Result, error) {
	if s.PutFile != "" {
		s.PutFile = env.Paths.File(s.PutFile)
	}
	return s.S3CpFile(a, b)
}
func (s *S3) S3CpFile(a, b string) (*goansible.Result, error) {
	if s.Region == "" {
		s.Region = "ap-northeast-1"
	}
	S3 := s3.New(session.New(), &aws.Config{Region: aws.String(s.Region)})

	res := goansible.NewResult(true)

	res.Add("bucket", s.Bucket)
	res.Add("remote", s.At)

	if s.PutFile != "" {

		f, err := os.Open(s.PutFile)
		if err != nil {
			return nil, err
		}

		if f == nil {
			return nil, fmt.Errorf("Unknown local file %s", s.PutFile)
		}

		defer f.Close()

		var perm string

		if s.Public {
			if s.Writable {
				perm = "public-read-write"
			} else {
				perm = "public-read"
			}
		} else {
			perm = "private"
		}

		ct := s.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}

		var po *s3.PutObjectOutput
		//	var pr *s3.PutObjectInput
		if s.GZip {
			_, po, err = s.ZipUploadReaderD(S3, s.At, f, ct, perm)
		} else {
			_, po, err = s.UploadFileD(S3, s.At, f, ct, perm)
		}
		if err != nil {
			return nil, err
		}

		//s.At: path ct:contentType, perm ,opts

		md5 := (*po.ETag)[1 : len(*po.ETag)-1]

		//pp.Print(pr)
		//res.Add("wrote", *pr.ContentLength)
		res.Add("local", s.PutFile)
		res.Add("md5sum", md5)

	} else if s.GetFile != "" {
		_, err := os.Stat(s.GetFile)
		if !os.IsNotExist(err) {
			f, err := os.Open(s.GetFile)
			if err != nil {
				return nil, err
			}
			md5hex, _, _, err := Md5Sum(f)
			f.Close()
			if err != nil {
				return nil, err
			}
			req := &s3.HeadObjectInput{
				Bucket: &s.Bucket,
				Key:    &s.At,
			}
			r, err := S3.HeadObject(req)
			if err != nil {
				return nil, err
			}
			if *r.ETag == `"`+md5hex+`"` {
				res.Changed = false
				res.Add("size", *r.ContentLength)
				res.Add("md5sum", md5hex)
				res.Add("local", s.GetFile)
				return res, nil
			}

		}
		if s.Mkdir {
			if err := os.MkdirAll(filepath.Dir(s.GetFile), 0755); err != nil {
				return nil, err
			}
		}

		req := &s3.GetObjectInput{
			Bucket: &s.Bucket,
			Key:    &s.At,
		}
		resGetObj, err := S3.GetObject(req)
		if err != nil {
			return nil, err
		}
		defer resGetObj.Body.Close()

		dist, err := os.OpenFile(s.GetFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}
		defer dist.Close()
		n, err := io.Copy(dist, resGetObj.Body)
		if err != nil {
			return nil, err
		}

		res.Add("read", n)
		res.Add("md5sum", (*resGetObj.ETag)[1:len(*resGetObj.ETag)-1])
		res.Add("local", s.GetFile)
	} else {
		return nil, fmt.Errorf("Specify put_file or get_file")
	}

	return res, nil
}

func init() {
	goansible.RegisterCommand("s3", &S3{})
}

func (s *S3) UploadFileD(S3 *s3.S3, key string, file *os.File, contentType, perm string) (*s3.PutObjectInput, *s3.PutObjectOutput, error) {

	md5hex, _, _, err := Md5Sum(file)
	if err != nil {
		return nil, nil, err
	}
	//s.Log.Debugf("key:%s md5=%s", key, md5hex)
	req := &s3.PutObjectInput{
		ACL:    &perm,
		Body:   file,
		Bucket: &s.Bucket,
		//ContentLength: &size,
		ContentType: &contentType,
		Key:         &key,
	}
	res, err := S3.PutObject(req)
	if err != nil {
		return req, res, err
	}
	if res == nil {
		return req, res, fmt.Errorf("res is nil pointer")
	}
	if res.ETag == nil {
		return req, res, fmt.Errorf("res.ETag is nil pointer")
	}
	if len(*res.ETag) < 2 {
		return req, res, fmt.Errorf("*res.ETag is too short. It should have 2 characters or more")
	}
	etag := (*res.ETag)[1 : len(*res.ETag)-1]
	if md5hex != etag {
		return req, res, fmt.Errorf("md5 and ETag does not match. md5:%s ETag:%s", md5hex, etag)
	}
	return req, res, err
}

func (s *S3) ZipUploadReaderD(S3 *s3.S3, key string, data io.Reader, contentType, perm string) (*s3.PutObjectInput, *s3.PutObjectOutput, error) {

	b := &bytes.Buffer{}
	gz := gzip.NewWriter(b)
	_, err := io.Copy(gz, data)
	if err != nil {
		return nil, nil, err
	}
	gz.Close()

	req := &s3.PutObjectInput{
		ACL:         &perm,                      // aws.StringValue   `xml:"-"` private | public-read | public-read-write | authenticated-read | bucket-owner-read | bucket-owner-full-control
		Body:        bytes.NewReader(b.Bytes()), // io.ReadSeeker     `xml:"-"`
		Bucket:      &s.Bucket,                  // aws.StringValue   `xml:"-"`
		ContentType: &contentType,               // aws.StringValue   `xml:"-"`
		Key:         &key,                       // aws.StringValue   `xml:"-"`
	}
	res, err := S3.PutObject(req)
	return req, res, err
}

func Md5sumBase64(data []byte) string {
	md5sum := md5.Sum(data)
	return base64.StdEncoding.EncodeToString(md5sum[:])
}
func Md5sumBase64File(f *os.File) (md5sum string, size int64, err error) {
	var offset int64
	offset, err = f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return
	}
	defer f.Seek(offset, os.SEEK_SET)
	h := md5.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return
	}
	md5sumRaw := h.Sum(nil)
	md5sum = base64.StdEncoding.EncodeToString(md5sumRaw[:])
	return
}
func Md5Sum(r io.ReadSeeker) (md5hex string, md5b64 string, size int64, err error) {
	var offset int64
	offset, err = r.Seek(0, os.SEEK_CUR)
	if err != nil {
		return
	}
	defer r.Seek(offset, os.SEEK_SET)
	digest := md5.New()
	size, err = io.Copy(digest, r)
	if err != nil {
		return
	}
	sum := digest.Sum(nil)
	md5hex = hex.EncodeToString(sum)
	md5b64 = base64.StdEncoding.EncodeToString(sum)
	return
}
