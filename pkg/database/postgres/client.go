package pg

import (
	"database/sql"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/rdsutils"

	//_ "github.com/jackc/pgx/v4/stdlib"
	_ "github.com/newrelic/go-agent/v3/integrations/nrpgx"
)

type Config struct {
	User               string
	Host               string
	Password           string
	Port               int
	DbName             string
	MaxOpenConnections int
	MaxIdleConnections int
}

// Get a DB connection pool using AWS IAM credentials
//
// https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.IAMDBAuth.Connecting.Go.html
func NewWithAwsIam(username, hostname, port, dbname string, config aws.Config) (*sql.DB, error) {
	// IMPORTANT: Only Supported on provisioned Aurora RDS clusters (not on Aurora Serverless)

	// Create an RDS client so we can grab the credential provider from it
	rdsClient := rds.New(config)
	credentials := rdsClient.Credentials
	region := rdsClient.Region

	// Generate IAM auth token (so we don't have to use a username/password)
	endpoint := fmt.Sprintf("%s:%s", hostname, port)
	authToken, err := rdsutils.BuildAuthToken(endpoint, region, username, credentials)
	if err != nil {
		return nil, err
	}

	// Use token based authentication
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s",
		hostname, port, username, authToken, dbname,
	)

	// Try to open a connection pool using the "pgx" driver (instead of "postgres")
	db, err := sql.Open("nrpgx", dsn)
	if err != nil {
		return nil, err
	}

	// Check if the connection was successful
	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Get a DB connection pool using username/password credentials
//
// https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.IAMDBAuth.Connecting.Go.html
func NewWithUsernameAndPassword(username, password, hostname, port, dbname string) (*sql.DB, error) {
	// IMPORTANT: Supported by Aurora Serverless clusters

	// Use password based authentication
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		username, password, hostname, port, dbname,
	)

	// TODO: either switch to IAM (non-serverless only) or enable SSL (download the db cert)
	// (https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/AuroraPostgreSQL.Security.html)

	// Try to open a connection pool using the "pgx" driver (instead of "postgres")
	db, err := sql.Open("nrpgx", dsn)
	if err != nil {
		return nil, err
	}

	// Check if the connection was successful
	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}
