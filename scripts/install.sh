#!/bin/bash

arch=`uname -m`

case $arch in
  x86_64 )
    arch="amd64" ;;
  486 | 586 | 686 )
    arch="386" ;;
  * )
    echo "Unsupported arch: $arch"
    exit 1
    ;;
esac

os=`uname`

case $os in
  Darwin )
    os="darwin"
    ;;
  Linux )
    os="linux"
    ;;
  * )
    echo "Unsupported os: $os"
    exit 1
esac

bin="goansible-$os-$arch"

echo "Determined your goansible binary to be: $bin"

cur=$(curl -s https://s3-us-west-2.amazonaws.com/goansible.vektra.io/release)

if test "$?" != "0"; then
  echo "Error computing current release"
  exit 1
fi

echo "Current release is: $cur"

url="https://s3-us-west-2.amazonaws.com/goansible.vektra.io/$cur/$bin"

echo "Downloading $url..."

curl --compressed -o goansible $url

chmod a+x goansible

echo ""
echo "Tachyon downloaded to current directory"
echo "We suggest you move it to somewhere in your PATH, like ~/bin"
echo ""
echo "Enjoy!"
