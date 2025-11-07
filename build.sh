#!/bin/bash
build_for_windows() {
    echo "🔨 Target: **Windows**"

    echo "Building client.exe ..."
    if CGO_CFLAGS="-O3" CGO_CXXFLAGS="-O3" GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ PKG_CONFIG_PATH=$HOME/mingw-libs/lib/pkgconfig  go build -o bin/client.exe -tags nolibopusfile -ldflags="-s -w -extldflags=-static" client/client.go ; then
        echo "✅ **Windows build successful**! Output: client.exe"
    else
        echo "❌ **Windows build failed**."
        exit 1
    fi


    echo "Building server.exe ..."
    if CGO_CFLAGS="-O3" CGO_CXXFLAGS="-O3" GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ PKG_CONFIG_PATH=$HOME/mingw-libs/lib/pkgconfig  go build -o bin/client.exe -tags nolibopusfile -ldflags="-s -w -extldflags=-static" server/server.go ; then
        echo "✅ **Windows build successful**! Output: server.exe"
    else
        echo "❌ **Windows build failed**."
        exit 1
    fi

}

build_for_linux() {
    echo "🔨 Target: **Linux**"
    
    echo "Building client ..."
    if CGO_ENABLED=1 go build -o bin/client client/client.go ; then
        echo "✅ **Linux build successful**! Output: client"
    else
        echo "❌ **Linux build failed**."
        exit 1
    fi


    echo "Building server ..."
    if CGO_ENABLED=1 go build -o bin/server server/server.go ; then
        echo "✅ **Linux build successful**! Output: server"
    else
        echo "❌ **Linux build failed**."
        exit 1
    fi
}

if [ -z "$1" ]; then
    echo "Usage: $0 <target_os>"
    echo "Please specify the target OS: **windows** or **linux**."
    exit 1
fi

TARGET_OS=$(echo "$1" | tr '[:upper:]' '[:lower:]') # Convert input to lowercase

# 2. Decide Target and Call Function
case "$TARGET_OS" in
    "windows")
        build_for_windows 
        ;;
    "linux")
        build_for_linux
        ;;
    *)
        echo "🛑 Invalid target specified: $1"
        echo "Please use either **windows** or **linux**."
        exit 1
        ;;
esac

exit 0