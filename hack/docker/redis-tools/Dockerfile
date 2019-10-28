FROM redis:5.0.4-alpine

COPY osm /usr/local/bin/osm
COPY redis-tools.sh /usr/local/bin/redis-tools.sh

ENTRYPOINT ["redis-tools.sh"]
