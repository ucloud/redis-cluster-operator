FROM redis:5.0.4

RUN set -x \
  && apt-get update \
  && apt-get install -y --no-install-recommends \
    ca-certificates \
    netcat \
  && rm -rf /var/lib/apt/lists/* /usr/share/doc /usr/share/man /tmp/*

COPY osm /usr/local/bin/osm
COPY redis-tools.sh /usr/local/bin/redis-tools.sh
RUN chmod +x /usr/local/bin/redis-tools.sh

ENTRYPOINT ["redis-tools.sh"]
