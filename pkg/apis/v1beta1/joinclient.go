/*
Copyright 2020 Mirantis, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1beta1

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/k0sproject/k0s/pkg/token"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"
)

// JoinClient is the client we can use to call k0s join APIs
type JoinClient struct {
	joinAddress string
	httpClient  http.Client
	bearerToken string
}

// JoinClientFromToken creates a new join api client from a token
func JoinClientFromToken(encodedToken string) (*JoinClient, error) {
	tokenBytes, err := token.JoinDecode(encodedToken)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode token")
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(tokenBytes)
	if err != nil {
		return nil, err
	}
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	ca := x509.NewCertPool()
	ca.AppendCertsFromPEM(config.CAData)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            ca,
	}
	tr := &http.Transport{TLSClientConfig: tlsConfig}
	c := &JoinClient{
		httpClient:  http.Client{Transport: tr},
		bearerToken: config.BearerToken,
	}
	c.joinAddress = config.Host
	logrus.Info("initialized join client successfully")
	return c, nil
}

// GetCA calls the CA sync API
func (j *JoinClient) GetCA() (CaResponse, error) {
	var caData CaResponse
	req, err := http.NewRequest(http.MethodGet, j.joinAddress+"/v1beta1/ca", nil)
	if err != nil {
		return caData, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", j.bearerToken))

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return caData, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return caData, fmt.Errorf("unexpected response status: %s", resp.Status)
	}
	logrus.Info("got valid CA response")
	if resp.Body == nil {
		return caData, fmt.Errorf("response body was nil !?!?")
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return caData, err
	}
	err = json.Unmarshal(b, &caData)
	if err != nil {
		return caData, err
	}
	return caData, nil
}

// JoinEtcd calls the etcd join API
func (j *JoinClient) JoinEtcd(peerAddress string) (EtcdResponse, error) {
	var etcdResponse EtcdResponse
	etcdRequest := EtcdRequest{
		PeerAddress: peerAddress,
	}
	name, err := os.Hostname()
	if err != nil {
		return etcdResponse, err
	}
	etcdRequest.Node = name

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(etcdRequest); err != nil {
		return etcdResponse, err
	}

	req, err := http.NewRequest(http.MethodPost, j.joinAddress+"/v1beta1/etcd/members", buf)
	if err != nil {
		return etcdResponse, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", j.bearerToken))
	resp, err := j.httpClient.Do(req)
	if err != nil {
		return etcdResponse, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return etcdResponse, fmt.Errorf("unexpected response status when trying to join etcd cluster: %s", resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return etcdResponse, err
	}
	err = json.Unmarshal(b, &etcdResponse)
	if err != nil {
		return etcdResponse, err
	}

	return etcdResponse, nil
}
