# Export only the structural schema of the running Windows R11 API.
# No employee values, names, tab numbers, dates, comments, or database path are written.
$ErrorActionPreference = 'Stop'
$uri = 'http://127.0.0.1:8779/api/state'

function Get-JsonType($Value) {
    if ($null -eq $Value) { return 'null' }
    if ($Value -is [string]) { return 'string' }
    if ($Value -is [bool]) { return 'boolean' }
    if ($Value -is [System.Array]) { return 'array' }
    if ($Value -is [System.Collections.IDictionary]) { return 'object' }
    if ($Value -is [System.ValueType]) { return $Value.GetType().Name }
    return 'object'
}

try {
    $response = Invoke-RestMethod -Uri $uri -Method Get -TimeoutSec 10
} catch {
    Write-Host 'Не удалось получить /api/state.' -ForegroundColor Red
    Write-Host 'Убедись, что LaborControl R11 запущен на этом компьютере Windows.'
    Write-Host 'Проверь, что локальный адрес 127.0.0.1:8779 доступен.'
    Write-Host $_.Exception.Message
    exit 1
}

if ($null -eq $response.data) {
    throw 'Ответ не содержит поля data. Файл отчёта не создан.'
}

$dataShape = [ordered]@{}
foreach ($property in $response.data.PSObject.Properties) {
    $value = $property.Value
    if ($value -is [System.Array]) {
        $sampleShapes = @()
        foreach ($item in @($value | Select-Object -First 2)) {
            if ($null -eq $item) {
                $sampleShapes += [ordered]@{ item_type = 'null'; fields = @() }
                continue
            }
            if ($item -is [System.Management.Automation.PSCustomObject]) {
                $fields = @()
                foreach ($field in $item.PSObject.Properties) {
                    $fields += [ordered]@{
                        name = $field.Name
                        type = Get-JsonType $field.Value
                    }
                }
                $sampleShapes += [ordered]@{ item_type = 'object'; fields = $fields }
            } else {
                $sampleShapes += [ordered]@{
                    item_type = Get-JsonType $item
                    fields = @()
                }
            }
        }
        $dataShape[$property.Name] = [ordered]@{
            type = 'array'
            count = $value.Count
            sample_shapes = $sampleShapes
        }
    } elseif ($value -is [System.Management.Automation.PSCustomObject]) {
        $fields = @()
        foreach ($field in $value.PSObject.Properties) {
            $fields += [ordered]@{
                name = $field.Name
                type = Get-JsonType $field.Value
            }
        }
        $dataShape[$property.Name] = [ordered]@{ type = 'object'; fields = $fields }
    } else {
        $dataShape[$property.Name] = [ordered]@{ type = Get-JsonType $value }
    }
}

$discrepancyCount = $null
if ($response.discrepancies -is [System.Array]) {
    $discrepancyCount = $response.discrepancies.Count
}

$report = [ordered]@{
    report = 'LaborControl R11 API schema only'
    captured_at_utc = [DateTime]::UtcNow.ToString('o')
    endpoint = $uri
    response_fields = @($response.PSObject.Properties.Name)
    data_shape = $dataShape
    discrepancies_type = Get-JsonType $response.discrepancies
    discrepancies_count = $discrepancyCount
}

$desktop = [Environment]::GetFolderPath('Desktop')
$outPath = Join-Path $desktop 'LaborControl_R11_schema.json'
$report | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $outPath -Encoding UTF8

Write-Host ''
Write-Host 'Готово. Сохранена только структура API, без значений и персональных данных.' -ForegroundColor Green
Write-Host $outPath
