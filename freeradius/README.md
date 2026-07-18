# FreeRADIUS host configuration

Mirrors of the live config on the server (secrets replaced with
`CHANGE_ME_SET_IN_ETC_FREERADIUS`). Needed to rebuild a droplet (e.g. Nairobi
prod). FreeRADIUS 3.2.x installed natively (`apt install freeradius freeradius-postgresql`).

| Repo file | Server path |
|---|---|
| `clients.conf` | `/etc/freeradius/3.0/clients.conf` |
| `mods-available/sql` | `/etc/freeradius/3.0/mods-enabled/sql` (real file, not symlink) |
| `sites-available/hotspot` | `/etc/freeradius/3.0/sites-enabled/hotspot` |

## Router (NAS) clients come from PostgreSQL

`mods-enabled/sql` has `read_clients = yes` / `client_table = "nas"` — routers
registered through the dashboard (which inserts into the `nas` table, migration
`005`) become RADIUS clients on the next FreeRADIUS restart.
`scripts/radius-sync.sh` runs from cron every minute
(`/etc/cron.d/myfibase-radius-sync`), and on any change to the `nas` table it
updates UFW (allow udp/1812-1813 per registered router IP, rules tagged
`myfibase-nas`) and restarts FreeRADIUS. `clients.conf` keeps only the
localhost test client.

## Per-router connection test

The dashboard's "Test connection" reads `radpostauth.nasipaddress`, a column
added by migration `005`. The stock postauth insert doesn't fill it — the
`post-auth` query in
`/etc/freeradius/3.0/mods-config/sql/main/postgresql/queries.conf` is patched
to include it:

```
query = "\
    INSERT INTO ${..postauth_table} \
        (username, pass, reply, authdate, nasipaddress ${..class.column_name}) \
    VALUES(\
        '%{User-Name}', \
        '%{%{User-Password}:-%{Chap-Password}}', \
        '%{reply:Packet-Type}', \
        '%S.%M', \
        '%{Packet-Src-IP-Address}' \
        ${..class.reply_xlat})"
```

## Cron entry

`/etc/cron.d/myfibase-radius-sync`:

```
* * * * * root /var/www/myfibase/scripts/radius-sync.sh >> /var/log/myfibase-radius-sync.log 2>&1
```
