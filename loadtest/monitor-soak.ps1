param(
    [int]$DurationMinutes = 35,
    [int]$DurationSeconds = 0,
    [int]$IntervalSeconds = 5,
    [string]$MetricsUrl = "http://127.0.0.1:8081/metrics",
    [string]$ContainerName = "relayim-app",
    [string]$OutputPath = "loadtest/reports/soak-monitor.csv",
    [string]$SummaryPath = "loadtest/reports/soak-monitor-summary.json"
)

$ErrorActionPreference = "Stop"

# metricValue 读取第一条匹配的 Prometheus 指标值
function metricValue(
    [string]$content,
    [string]$pattern,
    [double]$default = 0
) {
    $match = [regex]::Match($content, $pattern)
    if (-not $match.Success) {
        return $default
    }
    return [double]::Parse(
        $match.Groups[1].Value,
        [Globalization.CultureInfo]::InvariantCulture
    )
}

# metricSum 累加同名指标的所有标签值
function metricSum(
    [string]$content,
    [string]$pattern
) {
    $matches = [regex]::Matches($content, $pattern)
    $total = 0.0
    foreach ($match in $matches) {
        $total += [double]::Parse(
            $match.Groups[1].Value,
            [Globalization.CultureInfo]::InvariantCulture
        )
    }
    return $total
}

# dockerMemoryMiB 把 docker stats 的内存文本转换为 MiB
function dockerMemoryMiB([string]$memoryText) {
    $value = $memoryText.Split("/")[0].Trim()
    if ($value -match '^([0-9.]+)\s*GiB$') {
        return [double]$matches[1] * 1024
    }
    if ($value -match '^([0-9.]+)\s*MiB$') {
        return [double]$matches[1]
    }
    if ($value -match '^([0-9.]+)\s*KiB$') {
        return [double]$matches[1] / 1024
    }
    return 0
}

$outputDirectory = Split-Path -Parent $OutputPath
if ($outputDirectory) {
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
}
Remove-Item -LiteralPath $OutputPath -ErrorAction SilentlyContinue

$startedAt = Get-Date
$effectiveSeconds = if ($DurationSeconds -gt 0) {
    $DurationSeconds
} else {
    $DurationMinutes * 60
}
$deadline = $startedAt.AddSeconds($effectiveSeconds)
$sampleCount = 0
$errors = 0

while ((Get-Date) -lt $deadline) {
    $elapsedSeconds = [math]::Round(
        ((Get-Date) - $startedAt).TotalSeconds,
        1
    )
    $row = [ordered]@{
        timestamp_utc = (Get-Date).ToUniversalTime().ToString("o")
        elapsed_seconds = $elapsedSeconds
        available = 0
        goroutines = 0
        heap_bytes = 0
        rss_bytes = 0
        db_open = 0
        db_in_use = 0
        websocket_connections = 0
        outbox_pending = 0
        outbox_processing = 0
        outbox_dead = 0
        message_accepted = 0
        consumer_success = 0
        container_cpu_percent = 0
        container_memory_mib = 0
    }

    try {
        $content = (Invoke-WebRequest -UseBasicParsing `
            -Uri $MetricsUrl -TimeoutSec 10).Content
        $row.available = 1
        $row.goroutines = metricValue $content '(?m)^go_goroutines\s+([0-9.eE+\-]+)'
        $row.heap_bytes = metricValue $content '(?m)^go_memstats_alloc_bytes\s+([0-9.eE+\-]+)'
        $row.rss_bytes = metricValue $content '(?m)^process_resident_memory_bytes\s+([0-9.eE+\-]+)'
        $row.db_open = metricValue $content '(?m)^relayim_db_connections\{state="open"\}\s+([0-9.eE+\-]+)'
        $row.db_in_use = metricValue $content '(?m)^relayim_db_connections\{state="in_use"\}\s+([0-9.eE+\-]+)'
        $row.websocket_connections = metricValue $content '(?m)^relayim_websocket_connections\s+([0-9.eE+\-]+)'
        $row.outbox_pending = metricValue $content '(?m)^relayim_outbox_events\{status="pending"\}\s+([0-9.eE+\-]+)'
        $row.outbox_processing = metricValue $content '(?m)^relayim_outbox_events\{status="processing"\}\s+([0-9.eE+\-]+)'
        $row.outbox_dead = metricValue $content '(?m)^relayim_outbox_events\{status="dead"\}\s+([0-9.eE+\-]+)'
        $row.message_accepted = metricSum $content '(?m)^relayim_message_accepted_total\{[^}]*\}\s+([0-9.eE+\-]+)'
        $row.consumer_success = metricSum $content '(?m)^relayim_message_consumer_events_total\{event_type="message\.created\.v1",result="success"\}\s+([0-9.eE+\-]+)'
    } catch {
        $errors++
    }

    try {
        $stats = (docker stats --no-stream --format "{{.CPUPerc}};{{.MemUsage}}" $ContainerName).Trim()
        $parts = $stats.Split(";")
        if ($parts.Count -eq 2) {
            $row.container_cpu_percent = [double]($parts[0].TrimEnd("%"))
            $row.container_memory_mib = dockerMemoryMiB $parts[1]
        }
    } catch {
        $errors++
    }

    [pscustomobject]$row |
        Export-Csv -LiteralPath $OutputPath -Append -NoTypeInformation -Encoding utf8
    $sampleCount++
    Start-Sleep -Seconds $IntervalSeconds
}

$samples = Import-Csv -LiteralPath $OutputPath
$availableSamples = @($samples | Where-Object { $_.available -eq "1" })
$firstSample = $availableSamples | Select-Object -First 1
$lastSample = $availableSamples | Select-Object -Last 1
$summary = [ordered]@{
    started_at_utc = $startedAt.ToUniversalTime().ToString("o")
    duration_seconds = $effectiveSeconds
    interval_seconds = $IntervalSeconds
    sample_count = $sampleCount
    collection_errors = $errors
    max_goroutines = ($availableSamples | Measure-Object goroutines -Maximum).Maximum
    max_heap_mib = [math]::Round(
        (($availableSamples | Measure-Object heap_bytes -Maximum).Maximum / 1MB),
        2
    )
    max_rss_mib = [math]::Round(
        (($availableSamples | Measure-Object rss_bytes -Maximum).Maximum / 1MB),
        2
    )
    max_db_open = ($availableSamples | Measure-Object db_open -Maximum).Maximum
    max_db_in_use = ($availableSamples | Measure-Object db_in_use -Maximum).Maximum
    max_websocket_connections = ($availableSamples | Measure-Object websocket_connections -Maximum).Maximum
    max_outbox_pending = ($availableSamples | Measure-Object outbox_pending -Maximum).Maximum
    max_outbox_processing = ($availableSamples | Measure-Object outbox_processing -Maximum).Maximum
    max_outbox_dead = ($availableSamples | Measure-Object outbox_dead -Maximum).Maximum
    max_container_cpu_percent = ($availableSamples | Measure-Object container_cpu_percent -Maximum).Maximum
    max_container_memory_mib = ($availableSamples | Measure-Object container_memory_mib -Maximum).Maximum
    accepted_delta = [double]$lastSample.message_accepted - [double]$firstSample.message_accepted
    consumer_success_delta = [double]$lastSample.consumer_success - [double]$firstSample.consumer_success
}

$summary | ConvertTo-Json -Depth 5 |
    Set-Content -LiteralPath $SummaryPath -Encoding utf8
$summary | ConvertTo-Json -Depth 5
