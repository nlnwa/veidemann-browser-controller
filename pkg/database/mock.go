package database

import (
	frontierV1 "github.com/nlnwa/veidemann-api-go/frontier/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
	"os"
)

type Mock struct {
	*connection
}

// NewConnection creates a new connection object
func NewMock() *Mock {
	c := &Mock{connection: &connection{
		dbConnectOpts: r.ConnectOpts{
			NumRetries: 10,
		},
	}}
	c.dbSession = r.NewMock(c.dbConnectOpts)
	return c
}

// connect establishes connections
func (c *Mock) Connect() error {
	return nil
}

func (c *Mock) GetMock() *r.Mock {
	return c.dbSession.(*r.Mock)
}

func (c *Mock) WriteCrawlLog(crawlLog *frontierV1.CrawlLog) error {
	f, err := os.OpenFile("crawl.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()

	if _, err := f.WriteString(protojson.Format(crawlLog) + "\n"); err != nil {
		log.Println(err)
	}

	return c.connection.WriteCrawlLog(crawlLog)
}

func (c *Mock) WritePageLog(pageLog *frontierV1.PageLog) error {
	f, err := os.OpenFile("page.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()

	if _, err := f.WriteString(protojson.Format(pageLog) + "\n"); err != nil {
		log.Println(err)
	}

	return c.connection.WritePageLog(pageLog)
}
