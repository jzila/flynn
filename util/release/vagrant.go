package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/flynn/flynn/Godeps/_workspace/src/github.com/cupcake/goamz/aws"
	"github.com/flynn/flynn/Godeps/_workspace/src/github.com/cupcake/goamz/s3"
	"github.com/flynn/flynn/Godeps/_workspace/src/github.com/flynn/go-docopt"
)

func vagrant(args *docopt.Args) {
	auth, err := aws.EnvAuth()
	if err != nil {
		log.Fatal(err)
	}
	bucket := s3.New(auth, aws.USEast).Bucket(args.String["--bucket"])

	log.Println("fetching vagrant manifest")
	manifest, err := getVagrantManifest(bucket)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("getting list of boxes")
	list, err := bucket.List("vagrant/boxes/", "/", "", 0)
	if err != nil {
		log.Fatal(err)
	}
	for _, key := range list.Contents {
		log.Println("found vagrant box:", key.Key)
		url := bucket.URL(key.Key)
		if !manifest.includes(url) {
			log.Println("adding box to manifest:", key.Key)
			if err := manifest.add(key.Key); err != nil {
				log.Fatalf("Failed to add %s to manifest: %s", key.Key, err)
			}
		}
	}

	log.Println("saving manifest")
	if err := manifest.save(); err != nil {
		log.Fatal(err)
	}
}

var vagrantManifestPath = "vagrant/flynn-base.json"

func getVagrantManifest(bucket *s3.Bucket) (*Manifest, error) {
	manifest := &Manifest{Name: "flynn-base", bucket: bucket}

	body, err := bucket.GetReader(vagrantManifestPath)
	if err != nil {
		if s3Err, ok := err.(*s3.Error); ok && s3Err.Code == "NoSuchKey" {
			return manifest, nil
		}
		return nil, err
	}
	defer body.Close()

	if err := json.NewDecoder(body).Decode(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

type Manifest struct {
	Name     string     `json:"name"`
	Versions []*Version `json:"versions"`

	bucket *s3.Bucket
	urls   map[string]struct{}
}

func (m *Manifest) includes(url string) bool {
	if m.urls == nil {
		m.urls = make(map[string]struct{})
		for _, v := range m.Versions {
			for _, p := range v.Providers {
				m.urls[p.Url] = struct{}{}
			}
		}
	}
	_, ok := m.urls[url]
	return ok
}

func (m *Manifest) add(key string) error {
	res, err := m.bucket.GetResponse(key)
	if err != nil {
		return err
	}
	res.Body.Close()

	checksum := res.Header.Get("X-Amz-Meta-Sha256")
	if checksum == "" {
		return fmt.Errorf("Missing checksum header: %s", key)
	}

	version := res.Header.Get("X-Amz-Meta-Flynn-Version")
	if version == "" {
		return fmt.Errorf("Missing version header: %s", key)
	}

	providerName := res.Header.Get("X-Amz-Meta-Provider")
	if providerName == "" {
		return fmt.Errorf("Missing provider header: %s", key)
	}

	provider := &Provider{
		Name:         providerName,
		Url:          m.bucket.URL(key),
		Checksum:     checksum,
		ChecksumType: "sha256",
	}

	for _, v := range m.Versions {
		if v.Version == version {
			for _, p := range v.Providers {
				if p.Name == provider.Name {
					return fmt.Errorf("%s box already exists in manifest for version %s", p.Name, version)
				}
			}
			v.Providers = append(v.Providers, provider)
			return nil
		}
	}

	m.Versions = append(m.Versions, &Version{
		Version:   version,
		Providers: []*Provider{provider},
	})

	return nil
}

func (m *Manifest) save() error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := m.bucket.Put(vagrantManifestPath, body, "application/json", "public-read"); err != nil {
		return err
	}
	return nil
}

type Version struct {
	Version   string      `json:"version"`
	Providers []*Provider `json:"providers"`
}

type Provider struct {
	Name         string `json:"name"`
	Url          string `json:"url"`
	ChecksumType string `json:"checksum_type"`
	Checksum     string `json:"checksum"`
}
