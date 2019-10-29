#!/bin/bash
set -eou pipefail

# ref: https://github.com/kubedb/mysql/blob/master/hack/docker/mysql-tools/5.7.25/mysql-tools.sh

show_help() {
  echo "redis-tools.sh - run tools"
  echo " "
  echo "redis-tools.sh COMMAND [options]"
  echo " "
  echo "options:"
  echo "-h, --help                         show brief help"
  echo "    --data-dir=DIR                 path to directory holding db data (default: /var/data)"
  echo "    --host=HOST                    database host"
  echo "    --user=USERNAME                database username"
  echo "    --bucket=BUCKET                name of bucket"
  echo "    --folder=FOLDER                name of folder in bucket"
  echo "    --snapshot=SNAPSHOT            name of snapshot"
  echo "    --enable-analytics=ENABLE_ANALYTICS   send analytical events to Google Analytics (default false)"
}

RETVAL=0
DEBUG=${DEBUG:-}
REDIS_HOST=${REDIS_HOST:-}
REDIS_PORT=${REDIS_PORT:-6379}
REDIS_USER=${REDIS_USER:-}
REDIS_PASSWORD=${REDIS_PASSWORD:-}
REDIS_BUCKET=${REDIS_BUCKET:-}
REDIS_FOLDER=${REDIS_FOLDER:-}
REDIS_SNAPSHOT=${REDIS_SNAPSHOT:-}
REDIS_DATA_DIR=${REDIS_DATA_DIR:-/data}
OSM_CONFIG_FILE=/etc/osm/config
ENABLE_ANALYTICS=${ENABLE_ANALYTICS:-false}

op=$1
shift

while test $# -gt 0; do
  case "$1" in
    -h | --help)
      show_help
      exit 0
      ;;
    --data-dir*)
      export REDIS_DATA_DIR=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --host*)
      export REDIS_HOST=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --user*)
      export REDIS_USER=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --bucket*)
      export REDIS_BUCKET=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --folder*)
      export REDIS_FOLDER=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --snapshot*)
      export REDIS_SNAPSHOT=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --analytics* | --enable-analytics*)
      export ENABLE_ANALYTICS=$(echo $1 | sed -e 's/^[^=]*=//g')
      shift
      ;;
    --)
      shift
      break
      ;;
    *)
      show_help
      exit 1
      ;;
  esac
done

if [ -n "$DEBUG" ]; then
  env | sort | grep REDIS_*
  echo ""
fi

# Wait for redis to start
# ref: http://unix.stackexchange.com/a/5279
while ! nc -q 1 $REDIS_HOST $REDIS_PORT </dev/null; do
  echo "Waiting... database is not ready yet"
  sleep 5
done

# cleanup data dump dir
mkdir -p "$REDIS_DATA_DIR"
cd "$REDIS_DATA_DIR"
rm -rf *

case "$op" in
  backup)
    echo "Dumping database......"
    redis-cli --rdb dump.rdb -h ${REDIS_HOST} -a "${REDIS_PASSWORD}"

    echo "Uploading dump file to the backend......."
    osm push --enable-analytics="$ENABLE_ANALYTICS" --osmconfig="$OSM_CONFIG_FILE" -c "$REDIS_BUCKET" "$REDIS_DATA_DIR" "$REDIS_FOLDER/$REDIS_SNAPSHOT"

    echo "Backup successful"
    ;;
  restore)
    echo "Pulling backup file from the backend"
    osm pull --enable-analytics="$ENABLE_ANALYTICS" --osmconfig="$OSM_CONFIG_FILE" -c "$REDIS_BUCKET" "$REDIS_FOLDER/$REDIS_SNAPSHOT" "$REDIS_DATA_DIR"

    echo "Recovery successful"
    ;;
  *)
    (10)
    echo $"Unknown op!"
    RETVAL=1
    ;;
esac
exit "$RETVAL"
