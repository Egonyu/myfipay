-- FreeRADIUS standard tables
-- myFiBase writes to these when sessions are created/expired
-- FreeRADIUS reads from these to auth users at the router

CREATE TABLE radcheck (
    id        BIGSERIAL PRIMARY KEY,
    username  VARCHAR(64) NOT NULL DEFAULT '',
    attribute VARCHAR(64) NOT NULL DEFAULT '',
    op        CHAR(2)     NOT NULL DEFAULT '==',
    value     VARCHAR(253) NOT NULL DEFAULT ''
);
CREATE INDEX idx_radcheck_username ON radcheck(username, attribute);

CREATE TABLE radreply (
    id        BIGSERIAL PRIMARY KEY,
    username  VARCHAR(64) NOT NULL DEFAULT '',
    attribute VARCHAR(64) NOT NULL DEFAULT '',
    op        CHAR(2)     NOT NULL DEFAULT '=',
    value     VARCHAR(253) NOT NULL DEFAULT ''
);
CREATE INDEX idx_radreply_username ON radreply(username);

CREATE TABLE radgroupcheck (
    id        BIGSERIAL PRIMARY KEY,
    groupname VARCHAR(64) NOT NULL DEFAULT '',
    attribute VARCHAR(64) NOT NULL DEFAULT '',
    op        CHAR(2)     NOT NULL DEFAULT '==',
    value     VARCHAR(253) NOT NULL DEFAULT ''
);

CREATE TABLE radgroupreply (
    id        BIGSERIAL PRIMARY KEY,
    groupname VARCHAR(64) NOT NULL DEFAULT '',
    attribute VARCHAR(64) NOT NULL DEFAULT '',
    op        CHAR(2)     NOT NULL DEFAULT '=',
    value     VARCHAR(253) NOT NULL DEFAULT ''
);

CREATE TABLE radusergroup (
    username  VARCHAR(64) NOT NULL DEFAULT '',
    groupname VARCHAR(64) NOT NULL DEFAULT '',
    priority  INTEGER     NOT NULL DEFAULT 1
);
CREATE INDEX idx_radusergroup_username ON radusergroup(username);

CREATE TABLE radpostauth (
    id         BIGSERIAL PRIMARY KEY,
    username   VARCHAR(64)  NOT NULL,
    pass       VARCHAR(64)  NOT NULL DEFAULT '',
    reply      VARCHAR(32)  NOT NULL,
    authdate   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    class      VARCHAR(64)  DEFAULT NULL
);

CREATE TABLE radacct (
    radacctid           BIGSERIAL PRIMARY KEY,
    acctsessionid       VARCHAR(64)  NOT NULL DEFAULT '',
    acctuniqueid        VARCHAR(32)  NOT NULL DEFAULT '',
    username            VARCHAR(64)  NOT NULL DEFAULT '',
    realm               VARCHAR(64)  DEFAULT NULL,
    nasipaddress        INET         NOT NULL,
    nasportid           VARCHAR(15)  DEFAULT NULL,
    nasporttype         VARCHAR(32)  DEFAULT NULL,
    acctstarttime       TIMESTAMPTZ  DEFAULT NULL,
    acctupdatetime      TIMESTAMPTZ  DEFAULT NULL,
    acctstoptime        TIMESTAMPTZ  DEFAULT NULL,
    acctinterval        INTEGER      DEFAULT NULL,
    acctsessiontime     INTEGER      NOT NULL DEFAULT 0,
    acctauthentic       VARCHAR(32)  DEFAULT NULL,
    acctinputoctets     BIGINT       NOT NULL DEFAULT 0,
    acctoutputoctets    BIGINT       NOT NULL DEFAULT 0,
    calledstationid     VARCHAR(50)  NOT NULL DEFAULT '',
    callingstationid    VARCHAR(50)  NOT NULL DEFAULT '',
    acctterminatecause  VARCHAR(32)  NOT NULL DEFAULT '',
    servicetype         VARCHAR(32)  DEFAULT NULL,
    framedprotocol      VARCHAR(32)  DEFAULT NULL,
    framedipaddress     INET         DEFAULT NULL,
    class               VARCHAR(253) DEFAULT NULL
);

CREATE UNIQUE INDEX idx_radacct_unique   ON radacct(acctuniqueid);
CREATE INDEX idx_radacct_session         ON radacct(acctsessionid);
CREATE INDEX idx_radacct_username        ON radacct(username);
CREATE INDEX idx_radacct_start           ON radacct(acctstarttime);
CREATE INDEX idx_radacct_stop            ON radacct(acctstoptime);
