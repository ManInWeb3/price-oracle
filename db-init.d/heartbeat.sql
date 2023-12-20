CREATE TABLE IF NOT EXISTS heartbeat (
  "timestamp" timestamp NOT NULL default current_timestamp,
  "nodename" varchar(15) NOT NULL,
  PRIMARY KEY ("nodename")
);
