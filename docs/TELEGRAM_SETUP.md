# Telegram 봇 토큰·채팅 ID 설정 가이드

Chart Analyzer 프로젝트에서 알림을 받으려면 **Telegram Bot 토큰**과 **채팅 ID**가 필요합니다.  
Mac의 **Telegram Lite** (또는 공식 Telegram 앱)로 아래 순서대로 진행하면 됩니다.

---

## 1. 봇(Bot) 만들기 — 토큰 발급

### 1.1 BotFather 연동

1. Telegram Lite를 실행하고 검색창에 **`@BotFather`** 를 입력해 대화를 시작합니다.
2. **Start** 또는 **/start** 를 보냅니다.
3. 새 봇을 만들려면 **`/newbot`** 을 입력합니다.

### 1.2 봇 이름·사용자명 정하기

- **봇 이름 (Name)**  
  - 사람에게 보이는 이름입니다.  
  - 예: `Chart Analyzer Alerts`, `내 차트 알림봇` 등 자유롭게 정합니다.

- **봇 사용자명 (Username)**  
  - 반드시 `bot`으로 끝나야 합니다.  
  - 예: `my_chart_analyzer_bot`, `chart_alerts_123_bot`  
  - 이미 쓰인 이름이면 BotFather가 다른 이름을 요청합니다.

### 1.3 토큰 받기

설정이 끝나면 BotFather가 **긴 문자열**을 보냅니다. 형태는 다음과 비슷합니다:

```
1234567890:ABCdefGHIjklMNOpqrsTUVwxyz
```

이 값이 **봇 토큰(Bot Token)** 입니다.

- **절대 외부에 공개하거나 Git에 올리지 마세요.**  
- `.env` 파일의 `TELEGRAM_BOT_TOKEN=` 에만 넣어 사용합니다.

---

## 2. 채팅 ID(Chat ID) 알아내기

봇이 **어디로** 메시지를 보낼지 정하는 값이 **Chat ID**입니다.  
개인 채팅(나에게만)으로 받을 때와 그룹으로 받을 때 모두 사용할 수 있습니다.

### 방법 A: 봇에게 메시지 보낸 뒤 API로 확인 (추천)

1. 방금 만든 **봇**을 Telegram에서 검색해 대화를 시작합니다.
2. 아무 메시지나 한 줄 보냅니다. (예: `hello`)
3. 브라우저나 터미널에서 아래 URL을 엽니다.  
   **`YOUR_BOT_TOKEN`** 자리에 1단계에서 받은 토큰을 그대로 넣습니다.

   ```
   https://api.telegram.org/botYOUR_BOT_TOKEN/getUpdates
   ```

4. 열린 JSON 안에서 `"chat":{"id": 숫자}` 를 찾습니다.  
   - 개인 채팅: `"chat":{"id": 123456789, ...}` → **Chat ID는 `123456789`**  
   - 그룹: `"chat":{"id": -1001234567890, ...}` → **Chat ID는 `-1001234567890`** (음수)

5. (선택) 한 번 확인했으면 같은 URL에서 다시 `getUpdates`를 호출해도 되고, 나중에 봇이 메시지를 보낼 때 사용할 Chat ID만 `.env`에 저장하면 됩니다.

### 방법 B: @userinfobot 사용 (개인 ID만)

1. Telegram에서 **`@userinfobot`** 을 검색해 대화를 시작합니다.
2. **Start**를 누르면 봇이 당신의 **User ID**(숫자)를 알려줍니다.
3. **개인 채팅으로만** 봇 알림을 받을 경우, 이 숫자를 **Chat ID**로 사용하면 됩니다.  
   (그룹 Chat ID는 이 봇으로는 알 수 없고, 방법 A로 확인해야 합니다.)

---

## 3. 프로젝트에 반영하기

1. 프로젝트 루트의 **`.env`** 파일을 엽니다. (없으면 `.env.example`을 복사해 `.env` 생성)
2. 아래 두 줄에 값을 채웁니다:

   ```env
   TELEGRAM_BOT_TOKEN=1234567890:ABCdefGHIjklMNOpqrsTUVwxyz
   TELEGRAM_CHAT_ID=123456789
   ```

3. **그룹**으로 받을 때는 `TELEGRAM_CHAT_ID`에 그룹 Chat ID(보통 음수)를 넣습니다.  
   그룹에 봇을 추가한 뒤, 그룹에서 아무 메시지나 보내고 **방법 A**의 `getUpdates`로 확인한 값을 사용하면 됩니다.

4. `.env`는 Git에 올리지 않도록 이미 `.gitignore`에 포함되어 있는지 확인하세요.

---

## 4. 동작 확인

- 서버를 실행하면 로그에 **"Telegram 알림 활성화"** 가 찍힙니다.  
  (`TELEGRAM_BOT_TOKEN`과 `TELEGRAM_CHAT_ID`가 모두 설정된 경우)
- 룰 엔진에서 신호가 나오면 해당 Chat ID(개인 또는 그룹)로 메시지가 전송됩니다.

---

## 요약

| 항목 | 설명 |
|------|------|
| **TELEGRAM_BOT_TOKEN** | @BotFather에서 `/newbot`으로 봇 생성 후 받은 토큰 |
| **TELEGRAM_CHAT_ID** | 개인: @userinfobot 또는 getUpdates의 `chat.id` / 그룹: getUpdates의 `chat.id` (음수) |

- **Telegram Lite**에서도 BotFather, 봇 대화, getUpdates 결과로 확인하는 과정은 동일하게 할 수 있습니다.
- 토큰·Chat ID는 `.env`에만 두고, 문서나 코드 저장소에는 넣지 마세요.
