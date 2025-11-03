
# GoAudioStreamer

## Low-Latency Voice Chat Server

A simple, efficient Golang-based client-server application for voice communication during gaming sessions, designed for 2-4 friends with minimal bandwidth and resource usage.

### Project Story

This project was born out of necessity during a tough time. The last week of October, I came down with a severe flu — running a fever of 40°C (104°F)—which left me unable to focus on work or studies. To pass the time and stay connected, I turned to playing PC games with my friends. However, my aging PC struggled with third-party apps like ***Discord*** for voice chat; it caused noticeable FPS drops and consumed more bandwidth than necessary, making gameplay laggy and frustrating.

Frustrated with these issues, I decided to build a lightweight alternative: a terminal-based voice chat tool optimized for low latency and minimal resources. The goal was to enable seamless talking with 2-4 friends while gaming, without the overhead of bloated applications, reducing bandwidth to essentials and keeping CPU/GPU impact near zero.

### How It Works

This app uses a central server to handle audio mixing and distribution, with clients capturing microphone input and playing back received audio. Key features:

- ***Audio Handling***: Captures mono audio from the default microphone at 48kHz using PortAudio. Compresses it with the Opus codec at ~12kbps for low-bandwidth voice transmission.
- ***Server Mixing***: Up to 4 clients connect via TCP. The server decodes incoming audio, mixes frames (summing PCM samples with clipping to avoid distortion), re-encodes the mix, and broadcasts it back—only when 2+ clients are connected to save resources.
- ***Efficiency Features***: DTX (Discontinuous Transmission) skips silent/noisy frames to reduce data sent.
- Simple password authentication ensures only trusted friends join.
- ***Latency Reduction***: 20ms frames minimize delay; jitter buffering on clients smooths playback without underruns.

### Setup

Run the server with:

```bash
go run server.go -password=<pass>
```

Clients connect via:

```bash
go run client.go -host=<ip> -port=1234 -password=<pass>
```

Cross-compiled for Windows for easy sharing—no dependencies needed.

For full code and dependencies, see the source files. Contributions welcome to add features like auto-reconnect or UDP support!

## Cross compile for windows

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
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ PKG_CONFIG_PATH=$HOME/mingw-libs/lib/pkgconfig go build -o client.exe -tags nolibopusfile -ldflags="-s -w -extldflags=-static" client/client.go
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ PKG_CONFIG_PATH=$HOME/mingw-libs/lib/pkgconfig go build -o server.exe -tags nolibopusfile -ldflags="-s -w -extldflags=-static" server/server.go
```
