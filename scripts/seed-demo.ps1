param(
    [string]$BaseUrl = "http://127.0.0.1:8081",
    [string]$Suffix = (Get-Date -Format "MMddHHmmss")
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$BaseUrl = $BaseUrl.TrimEnd("/")
$Password = "Demo123456!"

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [string]$Token = "",
        [object]$Body = $null
    )

    $parameters = @{
        Method = $Method
        Uri    = "$BaseUrl/api/v1$Path"
    }
    if ($Token) {
        $parameters.Headers = @{
            Authorization = "Bearer $Token"
        }
    }
    if ($null -ne $Body) {
        $parameters.ContentType = "application/json"
        $parameters.Body = $Body | ConvertTo-Json -Depth 10 -Compress
    }
    return Invoke-RestMethod @parameters
}

function Register-DemoUser {
    param(
        [string]$Username,
        [string]$Nickname
    )

    $registered = Invoke-Api `
        -Method "POST" `
        -Path "/auth/register" `
        -Body @{
            username = $Username
            password = $Password
            confirm_password = $Password
            nickname = $Nickname
        }
    $login = Invoke-Api `
        -Method "POST" `
        -Path "/auth/login" `
        -Body @{
            username = $Username
            password = $Password
            device_name = "演示脚本"
            platform = 4
        }
    return [pscustomobject]@{
        id        = [string]$registered.id
        username  = $Username
        nickname  = $Nickname
        token     = [string]$login.token
        device_id = [string]$login.device_id
    }
}

function Add-DemoFriend {
    param(
        [object]$Requester,
        [object]$Recipient
    )

    Invoke-Api `
        -Method "POST" `
        -Path "/friend-requests" `
        -Token $Requester.token `
        -Body @{
            target_user_id = $Recipient.id
            remark = "RelayIM 演示好友"
        } | Out-Null

    $received = Invoke-Api `
        -Method "GET" `
        -Path "/friend-requests/received" `
        -Token $Recipient.token
    $request = $received.requests |
        Where-Object { [string]$_.user.id -eq [string]$Requester.id } |
        Select-Object -First 1
    if ($null -eq $request) {
        throw "未找到 $($Requester.username) 发给 $($Recipient.username) 的好友申请"
    }
    Invoke-Api `
        -Method "POST" `
        -Path "/friend-requests/$($request.id)/accept" `
        -Token $Recipient.token | Out-Null
}

function Send-DemoMessage {
    param(
        [object]$Sender,
        [string]$ConversationId,
        [string]$Content
    )

    return Invoke-Api `
        -Method "POST" `
        -Path "/conversations/$ConversationId/messages" `
        -Token $Sender.token `
        -Body @{
            client_message_id = [guid]::NewGuid().ToString()
            content = $Content
        }
}

function Accept-DemoGroupInvitation {
    param(
        [object]$Invitee,
        [string]$ConversationId
    )

    $received = Invoke-Api `
        -Method "GET" `
        -Path "/group-invitations" `
        -Token $Invitee.token
    $invitation = $received.invitations |
        Where-Object { [string]$_.conversation_id -eq $ConversationId } |
        Select-Object -First 1
    if ($null -eq $invitation) {
        throw "未找到 $($Invitee.username) 的群邀请"
    }
    Invoke-Api `
        -Method "POST" `
        -Path "/group-invitations/$($invitation.invitation_id)/accept" `
        -Token $Invitee.token | Out-Null
}

Write-Host "正在创建演示用户..."
$alice = Register-DemoUser "demo_alice_$Suffix" "演示用户阿岚"
$bob = Register-DemoUser "demo_bob_$Suffix" "演示用户北辰"
$carol = Register-DemoUser "demo_carol_$Suffix" "演示用户清禾"
$dave = Register-DemoUser "demo_dave_$Suffix" "演示用户远山"

Write-Host "正在建立好友关系..."
Add-DemoFriend $alice $bob
Add-DemoFriend $alice $carol
Add-DemoFriend $alice $dave

Write-Host "正在创建私聊和群聊..."
$direct = Invoke-Api `
    -Method "POST" `
    -Path "/conversations/private" `
    -Token $alice.token `
    -Body @{peer_user_id = $bob.id}
$privateMessage = Send-DemoMessage `
    $alice `
    $direct.id `
    "你好，这是一条可以直接演示的私聊消息。"

$group = Invoke-Api `
    -Method "POST" `
    -Path "/conversations/groups" `
    -Token $alice.token `
    -Body @{
        name = "RelayIM 演示群 $Suffix"
        avatar_url = ""
        member_user_ids = @($bob.id, $carol.id)
        invite_remark = "邀请参与 RelayIM 群聊演示"
    }
Invoke-Api `
    -Method "POST" `
    -Path "/conversations/$($group.id)/members" `
    -Token $alice.token `
    -Body @{
        member_user_ids = @($dave.id)
        invite_remark = "邀请参与 RelayIM 群管理员演示"
    } | Out-Null
Accept-DemoGroupInvitation $bob $group.id
Accept-DemoGroupInvitation $carol $group.id
Accept-DemoGroupInvitation $dave $group.id
Invoke-Api `
    -Method "PUT" `
    -Path "/conversations/$($group.id)/members/$($bob.id)/role" `
    -Token $alice.token `
    -Body @{role = 2} | Out-Null
Invoke-Api `
    -Method "PUT" `
    -Path "/conversations/$($group.id)/profile" `
    -Token $alice.token `
    -Body @{
        name = "RelayIM 演示群 $Suffix"
        avatar_url = "https://example.com/relayim-group.png"
    } | Out-Null

$groupWelcome = Send-DemoMessage `
    $alice `
    $group.id `
    "欢迎来到 RelayIM 演示群，这里可以演示群聊、实时消息、已读和群管理。"
$groupReply = Send-DemoMessage `
    $bob `
    $group.id `
    "收到，我已经被设置为群管理员。"
$recalled = Send-DemoMessage `
    $alice `
    $group.id `
    "这条消息将在演示数据初始化后撤回。"
Invoke-Api `
    -Method "DELETE" `
    -Path "/conversations/$($group.id)/messages/$($recalled.message_id)" `
    -Token $alice.token | Out-Null

Write-Host ""
Write-Host "演示数据创建完成。"
Write-Host "登录密码：$Password"
Write-Host "用户："
@($alice, $bob, $carol, $dave) |
    Format-Table username, nickname -AutoSize
Write-Host "私聊会话：$($direct.id)"
Write-Host "私聊消息：$($privateMessage.message_id)"
Write-Host "群聊会话：$($group.id)"
Write-Host "群聊消息：$($groupWelcome.message_id), $($groupReply.message_id)"
Write-Host "已撤回消息：$($recalled.message_id)"
Write-Host "打开：http://127.0.0.1:5173"
