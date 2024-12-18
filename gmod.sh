set -e

GREEN='\e[32m'
RED='\e[31m'
RESET='\e[0m'

OS=$(uname -s)
ARCH=$(uname -m)
UNAME="${OS}_${ARCH}"

echo -e "当前操作系统：$GREEN$UNAME$RESET"

function install() {
  URL=$(curl -s https://api.github.com/repos/xiaoxuan6/gmod/releases/latest| grep "browser_download_url" | grep "tar.gz" | cut -d '"' -f 4 | grep "$UNAME")
  if [ -z "$URL" ]; then
      echo -e "${RED}Unsupported platform: $UNAME${RESET}"
      exit 1
  fi


  curl -L -O "https://ghproxy.cn/$URL"
  FILENAME=$(echo "$URL" | cut -d '/' -f 9)
  if [ ! -f "$FILENAME" ]; then
      echo "url: $URL"
      echo -e "${RED}filename $FILENAME dose not exist${RESET}"
      exit 1
  fi

  tar xf "$FILENAME"
  rm -rf "$FILENAME"
  chmod +x gmod
  mv gmod /usr/local/bin/

  echo -e "${GREEN}gmod install done.${RESET}"
}

function uninstall() {
    rm -f /usr/local/bin/gmod
    echo -e "${GREEN}gmod uninstall successful!${RESET}"
}

case $1 in
install)
  install
  ;;
uninstall)
  uninstall
  ;;
*)
  echo "Not found $1 option"
  echo "Usage: $0 {install|uninstall}"
  echo ""
  exit 1
esac