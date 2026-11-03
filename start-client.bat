@echo off
chcp 65001 >nul 2>&1
setlocal
title 石器时代 客户端
cd /d "%~dp0"

rem ================== 服务器地址 ==================
rem 改这里就行；也可以用命令行参数覆盖：
rem   start-client.bat 192.168.1.5:7065
rem   start-client.bat wss://your-domain.example/ws
set "SERVER=127.0.0.1:7065"
rem ===============================================

if not "%~1"=="" set "SERVER=%~1"

rem Rust 工具链不在 PATH 时补上
set "CARGOBIN=%USERPROFILE%\.cargo\bin"
if exist "%CARGOBIN%\cargo.exe" set "PATH=%CARGOBIN%;%PATH%"

echo ==========================================
echo    石器时代 2.5   命令行客户端
echo ==========================================
echo.
echo    服务器 %SERVER%
echo.

netstat -ano | findstr "LISTENING" | findstr ":7065" >nul 2>&1
if errorlevel 1 echo    注意：本机 7065 没有服务端在监听；如果你连的是别的地址，忽略这句。

echo [1/2] 编译客户端...
cargo build --locked --manifest-path ".\client\Cargo.toml"
if errorlevel 1 goto BUILD_FAIL

echo.
echo [2/2] 启动客户端，窗口建议 120x40，退出后回到本窗口
echo.
"%~dp0client\target\debug\stoneage.exe" --server %SERVER% --no-update-check
echo.
echo 客户端已退出。
pause
exit /b 0

:BUILD_FAIL
echo.
echo *** 编译失败，请看上面的错误信息 ***
pause
exit /b 1
