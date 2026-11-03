@echo off
chcp 65001 >nul 2>&1
setlocal
title 石器时代 服务端 - 127.0.0.1:7065
cd /d "%~dp0"

echo ==========================================
echo    石器时代 2.5   本地服务端
echo ==========================================
echo.

rem ---- 找 go：优先用固定安装路径，找不到就退回 PATH ----
set "GOEXE=C:/Program Files\Go\bin\go.exe"
if not exist "%GOEXE%" set "GOEXE=go"

echo [1/3] 检查 7065 端口占用...
netstat -ano | findstr "LISTENING" | findstr ":7065" >nul 2>&1
if not errorlevel 1 echo    注意：7065 已在监听，可能已有服务端在跑，继续启动会失败。

echo [2/3] 编译服务端...
"%GOEXE%" build -o ".\build\stoneage-server.exe" ".\cmd\stoneage-server"
if errorlevel 1 goto BUILD_FAIL

echo [3/3] 启动服务端，按 Ctrl+C 停止
echo.
echo    游戏端口 127.0.0.1:7065
echo    管理端口 127.0.0.1:7066   （协议用例要用的本地管理口）
echo.
"%~dp0build\stoneage-server.exe" -admin-listen 127.0.0.1:7066 -protocol-log -
echo.
echo 服务端已退出。
pause
exit /b 0

:BUILD_FAIL
echo.
echo *** 编译失败，请看上面的错误信息 ***
pause
exit /b 1
