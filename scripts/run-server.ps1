[CmdletBinding()]
param(
    [string]$EnvFile = "",
    [string]$ServerAddr = "",
    [long]$SnowflakeWorkID = -1
)

$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($EnvFile)) {
    $EnvFile = Join-Path $projectRoot "internal\config\app.env"
}

if (-not (Test-Path -LiteralPath $EnvFile)) {
    throw "未找到应用环境变量文件：$EnvFile。请先复制 internal\config\app.env.example。"
}

# 只解析 KEY=VALUE 形式，不执行文件中的内容，避免把配置误当成脚本。
$environment = @{}
foreach ($line in Get-Content -LiteralPath $EnvFile) {
    $trimmed = $line.Trim()
    if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
        continue
    }

    $parts = $trimmed.Split("=", 2)
    if ($parts.Count -ne 2) {
        throw "无效的环境变量行：$line"
    }

    $name = $parts[0].Trim()
    if ($name -notmatch "^[A-Za-z_][A-Za-z0-9_]*$") {
        throw "无效的环境变量名：$name"
    }

    $value = $parts[1].Trim()
    if ($value.Length -ge 2) {
        $first = $value[0]
        $last = $value[$value.Length - 1]
        if (($first -eq '"' -and $last -eq '"') -or
            ($first -eq "'" -and $last -eq "'")) {
            $value = $value.Substring(1, $value.Length - 2)
        }
    }

    $environment[$name] = $value
}

if (-not [string]::IsNullOrWhiteSpace($ServerAddr)) {
    $environment["GINCHAT_SERVER_ADDR"] = $ServerAddr.Trim()
}
if ($SnowflakeWorkID -ge 0) {
    $environment["GINCHAT_SNOWFLAKE_WORK_ID"] = [string]$SnowflakeWorkID
}

# 环境变量只注入即将启动的 Go 子进程，不污染当前 PowerShell 会话。
$goCommand = Get-Command go -ErrorAction Stop
$process = Start-Process `
    -FilePath $goCommand.Source `
    -ArgumentList @("run", "./cmd/server") `
    -WorkingDirectory $projectRoot `
    -NoNewWindow `
    -Wait `
    -PassThru `
    -Environment $environment

exit $process.ExitCode
