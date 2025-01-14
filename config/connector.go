package config

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/viant/datly/shared"
	"github.com/viant/scy"
)

//Connector represents database/sql named connection config
type Connector struct {
	shared.Reference
	Secret *scy.Resource
	Name   string
	Driver string
	DSN    string
	//TODO add secure password storage
	db          *sql.DB
	initialized bool
}

//Init initializes connector. It is possible to inherit from other Connector using Ref field.
//If Ref is specified, then Connector with the name has to be registered in Connectors
func (c *Connector) Init(ctx context.Context, connectors Connectors) error {
	if c.Ref != "" {
		connector, err := connectors.Lookup(c.Ref)
		if err != nil {
			return err
		}
		c.inherit(connector)
	}

	if err := c.Validate(); err != nil {
		return err
	}

	db, err := c.Db()
	if err != nil {
		return err
	}

	err = db.PingContext(ctx)
	if err != nil {
		return err
	}

	c.initialized = true
	return nil
}

//Db creates connection to the DB.
//It is important to not close the DB since the connection is shared.
func (c *Connector) Db() (*sql.DB, error) {
	if c.db != nil {
		return c.db, nil
	}

	var err error
	dsn := c.DSN
	if c.Secret != nil {
		secrets := scy.New()
		secret, err := secrets.Load(context.Background(), c.Secret)
		if err != nil {
			return nil, err
		}

		dsn = secret.Expand(dsn)
	}

	c.db, err = sql.Open(c.Driver, dsn)
	return c.db, err
}

//Validate check if connector was configured properly.
//Name, Driver and DSN are required.
func (c *Connector) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("connector name was empty")
	}

	if c.Driver == "" {
		return fmt.Errorf("connector driver was empty")
	}

	if c.DSN == "" {
		return fmt.Errorf("connector dsn was empty")
	}
	return nil
}

func (c *Connector) inherit(connector *Connector) {
	if c.DSN == "" {
		c.DSN = connector.DSN
	}

	if c.Driver == "" {
		c.Driver = connector.Driver
	}

	if c.DSN == "" {
		c.DSN = connector.DSN
	}

	if c.db == nil {
		c.db = connector.db
	}

	if c.Name == "" {
		c.Name = connector.Name
	}
}
