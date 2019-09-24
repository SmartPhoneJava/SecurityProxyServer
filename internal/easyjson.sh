#!/bin/sh
echo ""
echo "  --------------------------"
echo "  -----Easyjson generate----"
echo "  --------------------------"
echo ""
trap 'echo " stop" ' INT TERM
#chmod +x easyjson.sh && ./easyjson.sh

# set GOPATH and PATH
export GOPATH=$HOME/go
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
done=0

# install easyjson
go get -u github.com/mailru/easyjson/...
#apt install golang-easyjson
go mod tidy
go get -u
go mod vendor

echo "  1. Copy project to GOPATH"
# we need THISDIR to return back at the end
export THISDIR=$PWD
export PATHDIR=$GOPATH/src/github.com/SmartPhoneJava/SecurityProxyServer
# create folder, -p - create parents folders
mkdir -p $PATHDIR && \
# -r - copy folder

cp -r $THISDIR/models.go $PATHDIR && \

echo "  2. Apply easyjson to models" && \
export MODELSPATH=$PATHDIR && \
cd $PATHDIR && \
easyjson . && \
cp $PATHDIR/models_easyjson.go $THISDIR && \
done=1

echo "  3. Remove project from GOPATH"
rm -R $PATHDIR
cd $THISDIR

echo ""
if [ "$done" -eq 1 ]
then 
echo "  ----------Done!-----------"
else
echo "  ----------Error!-----------"
exit 1
fi