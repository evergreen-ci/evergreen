package jiffybox

import (
	"strconv"
	"time"
)

type Backup struct {
	ServerId int
	Key      string
	Id       string `json:"id"`
	Created  int64  `json:"created"`
}

func (backup *Backup) CreatedAt() time.Time {
	return time.Unix(backup.Created, 0)
}

type BackupsResponse struct {
	Message []*Message                    `json:"message"`
	Result  map[string]map[string]*Backup `json:"result"`
}

func (client *Client) CreateBackup(id int) error {
	httpResponse, e := client.PostForm("backups/"+strconv.Itoa(id), nil)
	defer httpResponse.Response.Body.Close()
	if e != nil {
		return e
	}
	defer httpResponse.Response.Body.Close()
	rsp := &ErrorResponse{}
	e = client.unmarshal(httpResponse.Content, rsp)
	if e != nil {
		return e
	}
	return nil
}

func (client *Client) Backups() (backups []*Backup, e error) {
	rsp := &BackupsResponse{}
	e = client.LoadResource("backups", rsp)
	if e != nil {
		return backups, e
	}
	for server, backupsHash := range rsp.Result {
		for key, backup := range backupsHash {
			backup.ServerId, _ = strconv.Atoi(server)
			backup.Key = key
			backups = append(backups, backup)
		}
	}
	return backups, e
}

type BackupsForServerResponse struct {
	Message []*Message         `json:"message"`
	Result  map[string]*Backup `json:"result"`
}

func (client *Client) BackupsForServer(id int) (backups []*Backup, e error) {
	rsp := BackupsForServerResponse{}
	e = client.LoadResource(client.BaseUrl()+"/backups/"+strconv.Itoa(id), rsp)
	if e != nil {
		return backups, e
	}
	for key, backup := range rsp.Result {
		backup.ServerId = id
		backup.Key = key
		backups = append(backups, backup)
	}
	return backups, e
}
