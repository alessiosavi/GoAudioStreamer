# CROSS COMPILE FOR WINDOWS

```bash

sudo apt install libportaudio2 libportaudiocpp0 libportaudio-ocaml-dev  libopus-dev libasound2-dev libogg-dev libopusfile-dev libopus-dev libasound2-dev git build-essential pkg-config cmake autoconf libtool gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64 mingw-w64-tools mingw-w64-x86-64-dev  -y

mkdir $HOME/mingw-libs
mkdir -p /opt/SP/misc
cd $_
wget http://files.portaudio.com/archives/pa_stable_v190700_20210406.tgz && tar -xzf pa_stable_v190700_20210406.tgz && rm pa_stable_v190700_20210406.tgz && cd portaudio
./configure --host=x86_64-w64-mingw32 --enable-static --disable-shared --prefix=$HOME/mingw-libs
make -j$(nproc)
make install
cd ..

wget https://downloads.xiph.org/releases/ogg/libogg-1.3.6.tar.gz && tar -xzf libogg-1.3.6.tar.gz && rm libogg-1.3.6.tar.gz && cd libogg-1.3.6
./configure --host=x86_64-w64-mingw32 --enable-static --disable-shared --prefix=$HOME/mingw-libs
make -j$(nproc)
make install
cd ..

wget https://downloads.xiph.org/releases/opus/opus-1.5.2.tar.gz && tar -xzf opus-1.5.2.tar.gz  && rm  opus-1.5.2.tar.gz  && cd opus-1.5.2
./configure --host=x86_64-w64-mingw32 --enable-static --disable-shared --with-winapi=wasapi,wmme,directsound --prefix=$HOME/mingw-libs
make -j$(nproc)
make install
cd ..


# cd into GoAudioStreamer
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ PKG_CONFIG_PATH=$HOME/mingw-libs/lib/pkgconfig  go build -o client.exe -tags nolibopusfile -ldflags="-s -w -extldflags=-static" client/client.go
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ PKG_CONFIG_PATH=$HOME/mingw-libs/lib/pkgconfig  go build -o server.exe -tags nolibopusfile -ldflags="-s -w -extldflags=-static" server/server.go
```
