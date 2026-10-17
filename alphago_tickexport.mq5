//+------------------------------------------------------------------+
//|                                              AlphaGo_Listener.mq5 |
//|                                   Copyright 2026, AlphaGo Trading |
//+------------------------------------------------------------------+
#property copyright "AlphaGo Trading System"
#property link      "https://github.com/nes224/alphago-mt5"
#property version   "1.30"
#property strict

// v1.30
//  - เพิ่มคำสั่ง RATES: ขอแท่งเทียนย้อนหลัง (ทุก TF) ให้ Gateway เติม gap เอง
//  - tick ส่ง time_msc (มิลลิวินาที) + timestamp ใช้เวลาของ tick จริง แทน TimeCurrent()
//  - SendAll(): ส่งข้อมูลให้ครบทุก byte บน non-blocking socket
//    (ของเดิมเรียก send() ครั้งเดียว ถ้าส่งได้ไม่ครบ JSON จะขาดกลางบรรทัด)

#include <Trade\Trade.mqh>

//--- Import Winsock API from ws2_32.dll
#import "ws2_32.dll"
   int WSAStartup(ushort wVersionRequested, uchar &lpWSAData[]);
   int WSACleanup();
   int WSAGetLastError();
   uint socket(int af, int type, int protocol);
   int bind(uint s, uchar &name[], int namelen);
   int listen(uint s, int backlog);
   uint accept(uint s, uchar &addr[], int &addrlen);
   int recv(uint s, uchar &buf[], int len, int flags);
   int send(uint s, uchar &buf[], int len, int flags);
   int closesocket(uint s);
   int ioctlsocket(uint s, long cmd, uint &argp);
#import

//--- Constants
#define AF_INET            2
#define SOCK_STREAM        1
#define IPPROTO_TCP        6
#define FIONBIO            0x8004667E
#define INVALID_SOCKET_VAL 0xFFFFFFFF
#define WSAEWOULDBLOCK     10035

//--- Inputs
input ushort InpCommandPort      = 5555; // Port สำหรับ Order Execution
input ushort InpStreamPort       = 5556; // Port สำหรับ Real-time Price Stream
input uint   InpMaxBuffer        = 4096;
input int    InpMaxRatesPerReply = 200;  // RATES: จำนวนแท่งสูงสุดต่อ 1 คำตอบ
input uint   InpCmdSendTimeoutMs    = 3000; // timeout ส่งคำตอบ command
input uint   InpStreamSendTimeoutMs = 200;  // timeout ส่ง tick (สั้น เพื่อไม่ให้ OnTick ค้าง)

//--- Globals
CTrade g_trade;
uint   g_cmd_server_socket  = INVALID_SOCKET_VAL;
uint   g_cmd_client_socket  = INVALID_SOCKET_VAL;

uint   g_stream_server_socket = INVALID_SOCKET_VAL;
uint   g_stream_client_socket = INVALID_SOCKET_VAL;

uchar  g_rx_buffer[];

// Helper Bind Socket
uint CreateAndBindSocket(ushort port)
{
   uint sock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
   if(sock == INVALID_SOCKET_VAL) return INVALID_SOCKET_VAL;

   uint nonblock = 1;
   ioctlsocket(sock, FIONBIO, nonblock);

   uchar sockaddr[16];
   ArrayInitialize(sockaddr, 0);
   sockaddr[0] = AF_INET & 0xFF;
   sockaddr[1] = (AF_INET >> 8) & 0xFF;
   sockaddr[2] = (uchar)((port >> 8) & 0xFF);
   sockaddr[3] = (uchar)(port & 0xFF);

   if(bind(sock, sockaddr, 16) != 0 || listen(sock, 5) != 0)
   {
      closesocket(sock);
      return INVALID_SOCKET_VAL;
   }
   return sock;
}

//+------------------------------------------------------------------+
//| ส่ง text ให้ครบทุก byte บน non-blocking socket                     |
//| send() อาจส่งได้แค่บางส่วน หรือคืน WSAEWOULDBLOCK ตอน buffer เต็ม  |
//| → วนส่งส่วนที่เหลือจนครบ หรือจนหมดเวลา                            |
//| คืน false = ส่งไม่สำเร็จ (ผู้เรียกควรปิด socket นั้น)              |
//+------------------------------------------------------------------+
bool SendAll(uint sock, string text, uint timeout_ms)
{
   uchar data[];
   int total = StringToCharArray(text, data, 0, WHOLE_ARRAY, CP_UTF8) - 1; // ตัด '\0' ท้าย
   if(total <= 0) return true;

   uchar chunk[];
   int   sent  = 0;
   uint  start = GetTickCount();

   while(sent < total)
   {
      int remaining = total - sent;
      int n;
      if(sent == 0)
         n = send(sock, data, remaining, 0);
      else
      {
         ArrayResize(chunk, remaining);
         ArrayCopy(chunk, data, 0, sent, remaining);
         n = send(sock, chunk, remaining, 0);
      }

      if(n > 0)
      {
         sent += n;
         continue;
      }

      int err = WSAGetLastError();
      if(err != WSAEWOULDBLOCK)
      {
         PrintFormat("send() failed, WSA error %d", err);
         return false;
      }
      if(GetTickCount() - start > timeout_ms)
      {
         PrintFormat("send() timeout after %u ms (%d/%d bytes)", timeout_ms, sent, total);
         return false;
      }
      Sleep(1);
   }
   return true;
}

void CloseStreamClient(string reason)
{
   if(g_stream_client_socket == INVALID_SOCKET_VAL) return;
   Print("Go Stream Client closed: ", reason);
   closesocket(g_stream_client_socket);
   g_stream_client_socket = INVALID_SOCKET_VAL;
}

void CloseCmdClient(string reason)
{
   if(g_cmd_client_socket == INVALID_SOCKET_VAL) return;
   Print("Go Command Client closed: ", reason);
   closesocket(g_cmd_client_socket);
   g_cmd_client_socket = INVALID_SOCKET_VAL;
}

//+------------------------------------------------------------------+
//| Expert initialization function                                   |
//+------------------------------------------------------------------+
int OnInit()
{
   // รีเซ็ต client socket handle ให้สะอาดทุกครั้งที่ OnInit() รัน — กันเคสที่
   // ตัวแปร global เก็บค่าเก่าจาก session ก่อนหน้าค้างอยู่ (ถ้าไม่รีเซ็ต
   // เงื่อนไข accept() ใน OnTimer() จะไม่มีวันเป็นจริงเลย เพราะคิดว่ามี
   // client เก่าเชื่อมต่ออยู่แล้วทั้งที่จริงๆ ไม่มี)
   g_cmd_client_socket    = INVALID_SOCKET_VAL;
   g_stream_client_socket = INVALID_SOCKET_VAL;

   ArrayResize(g_rx_buffer, (int)InpMaxBuffer);
   g_trade.SetTypeFilling(ORDER_FILLING_IOC);

   uchar wsaData[400];
   if(WSAStartup(0x0202, wsaData) != 0)
   {
      Print("WSAStartup failed.");
      return(INIT_FAILED);
   }

   // 1. Setup Command Socket (Port 5555)
   g_cmd_server_socket = CreateAndBindSocket(InpCommandPort);
   if(g_cmd_server_socket == INVALID_SOCKET_VAL)
   {
      Print("Failed to bind Command Socket on port ", InpCommandPort);
      WSACleanup();
      return(INIT_FAILED);
   }

   // 2. Setup Stream Socket (Port 5556)
   g_stream_server_socket = CreateAndBindSocket(InpStreamPort);
   if(g_stream_server_socket == INVALID_SOCKET_VAL)
   {
      Print("Failed to bind Stream Socket on port ", InpStreamPort);
      closesocket(g_cmd_server_socket);
      WSACleanup();
      return(INIT_FAILED);
   }

   PrintFormat("AlphaGo EA v1.30 Ready! Command Port: %d | Stream Port: %d", InpCommandPort, InpStreamPort);
   EventSetMillisecondTimer(10);
   return(INIT_SUCCEEDED);
}

//+------------------------------------------------------------------+
//| Expert deinitialization function                                 |
//+------------------------------------------------------------------+
void OnDeinit(const int reason)
{
   EventKillTimer();

   if(g_cmd_client_socket != INVALID_SOCKET_VAL) closesocket(g_cmd_client_socket);
   if(g_cmd_server_socket != INVALID_SOCKET_VAL) closesocket(g_cmd_server_socket);

   if(g_stream_client_socket != INVALID_SOCKET_VAL) closesocket(g_stream_client_socket);
   if(g_stream_server_socket != INVALID_SOCKET_VAL) closesocket(g_stream_server_socket);

   WSACleanup();
   Print("AlphaGo EA stopped.");
}

//+------------------------------------------------------------------+
//| OnTick Event: Stream Real-time Price Data to Go Client           |
//+------------------------------------------------------------------+
void OnTick()
{
   if(g_stream_client_socket == INVALID_SOCKET_VAL)
      return;

   MqlTick tick;
   if(!SymbolInfoTick(_Symbol, tick))
      return;

   // timestamp: เวลาของ tick จริง (เดิมใช้ TimeCurrent()) รูปแบบเดิม — ของเก่าไม่พัง
   // time_msc : เวลาของ tick เป็นมิลลิวินาที (เวลา server ของ broker)
   string json_tick = StringFormat(
      "{\"type\":\"tick\",\"symbol\":\"%s\",\"bid\":%.5f,\"ask\":%.5f,\"volume\":%I64d,\"timestamp\":\"%s\",\"time_msc\":%I64d}\n",
      _Symbol, tick.bid, tick.ask, (long)tick.volume,
      TimeToString(tick.time, TIME_DATE|TIME_SECONDS), (long)tick.time_msc
   );

   if(!SendAll(g_stream_client_socket, json_tick, InpStreamSendTimeoutMs))
      CloseStreamClient("send failed");
}

//+------------------------------------------------------------------+
//| OnTradeTransaction: จับตอน Position ปิด (SL/TP/ปิดมือ) ส่งผล    |
//| Win/Loss จริงกลับไปให้ Go ผ่าน Stream Socket                    |
//+------------------------------------------------------------------+
void OnTradeTransaction(const MqlTradeTransaction &trans,
                         const MqlTradeRequest &request,
                         const MqlTradeResult &result)
{
   // สนใจแค่ตอนมี "deal" ใหม่เกิดขึ้น (การจับคู่ order กับราคาจริงในตลาด)
   if(trans.type != TRADE_TRANSACTION_DEAL_ADD)
      return;

   ulong deal_ticket = trans.deal;
   if(!HistoryDealSelect(deal_ticket))
      return;

   // DEAL_ENTRY_OUT = deal ที่ "ปิด" position (ไม่ใช่ deal ที่เปิด position ใหม่)
   long entry = HistoryDealGetInteger(deal_ticket, DEAL_ENTRY);
   if(entry != DEAL_ENTRY_OUT)
      return;

   string symbol = HistoryDealGetString(deal_ticket, DEAL_SYMBOL);
   double profit = HistoryDealGetDouble(deal_ticket, DEAL_PROFIT)
                  + HistoryDealGetDouble(deal_ticket, DEAL_SWAP)
                  + HistoryDealGetDouble(deal_ticket, DEAL_COMMISSION);
   ulong position_id = HistoryDealGetInteger(deal_ticket, DEAL_POSITION_ID);

   if(g_stream_client_socket != INVALID_SOCKET_VAL)
   {
      string json_event = StringFormat(
         "{\"type\":\"trade_closed\",\"symbol\":\"%s\",\"ticket\":%I64u,\"profit\":%.2f,\"timestamp\":\"%s\"}\n",
         symbol, position_id, profit, TimeToString(TimeCurrent(), TIME_DATE|TIME_SECONDS)
      );

      if(SendAll(g_stream_client_socket, json_event, InpCmdSendTimeoutMs))
         PrintFormat("Trade Closed Event Sent: %s Ticket=%I64u Profit=%.2f", symbol, position_id, profit);
      else
         CloseStreamClient("send failed (trade_closed)");
   }
}

//+------------------------------------------------------------------+
//| Timer Event: Handle Incoming Socket Connections & Commands       |
//+------------------------------------------------------------------+
void OnTimer()
{
   uchar addr[16];
   int addr_len = 16;

   // 1. Accept Command Client (Port 5555)
   if(g_cmd_client_socket == INVALID_SOCKET_VAL)
   {
      uint sock = accept(g_cmd_server_socket, addr, addr_len);
      if(sock != INVALID_SOCKET_VAL && sock != 0)
      {
         g_cmd_client_socket = sock;
         uint nonblock = 1;
         ioctlsocket(g_cmd_client_socket, FIONBIO, nonblock);
         Print("Go Command Client connected (Port 5555)!");
      }
   }

   // 2. Accept Stream Client (Port 5556)
   if(g_stream_client_socket == INVALID_SOCKET_VAL)
   {
      uint sock = accept(g_stream_server_socket, addr, addr_len);
      if(sock != INVALID_SOCKET_VAL && sock != 0)
      {
         g_stream_client_socket = sock;
         uint nonblock = 1;
         ioctlsocket(g_stream_client_socket, FIONBIO, nonblock);
         Print("Go Stream Client connected (Port 5556)!");
      }
   }

   // 2.5 ตรวจจับ Dead Stream Client — Stream เป็น one-way (EA->Go) เลยไม่มีใคร
   //     เช็ค recv() มาก่อน ทำให้ตอน Go ฝั่งนั้นหลุดไป g_stream_client_socket
   //     ไม่เคยถูกเคลียร์ ค้างเป็น client เก่าตลอดกาล ไม่ accept() connection ใหม่อีกเลย
   if(g_stream_client_socket != INVALID_SOCKET_VAL)
   {
      uchar peek_buf[16];
      int len = recv(g_stream_client_socket, peek_buf, 16, 0);
      if(len == 0)
         CloseStreamClient("disconnected");
      // len < 0 บน non-blocking socket ปกติคือ WSAEWOULDBLOCK (ไม่มีข้อมูลเข้ามา) — ไม่ใช่ error
   }

   // 3. Read Command Data from Port 5555
   //    หมายเหตุ: 1 ครั้งของ recv() = 1 คำสั่ง → ฝั่ง Go ต้องส่งทีละคำสั่งแล้วรอคำตอบก่อน
   if(g_cmd_client_socket != INVALID_SOCKET_VAL)
   {
      int len = recv(g_cmd_client_socket, g_rx_buffer, (int)InpMaxBuffer, 0);
      if(len > 0)
      {
         string json_req = CharArrayToString(g_rx_buffer, 0, len, CP_UTF8);
         StringTrimRight(json_req);
         StringTrimLeft(json_req);

         if(StringLen(json_req) > 0)
         {
            string action = ExtractJsonValue(json_req, "action");
            if(action != "RATES") // RATES ถูกเรียกบ่อยตอนเติม gap — ไม่ต้อง log ทุกครั้ง
               Print("Received Command: ", json_req);

            string json_resp = ProcessTradeCommand(json_req);
            if(!SendAll(g_cmd_client_socket, json_resp, InpCmdSendTimeoutMs))
               CloseCmdClient("send failed");
         }
      }
      else if(len == 0)
      {
         CloseCmdClient("disconnected");
      }
   }
}

//+------------------------------------------------------------------+
//| Process Trade Command                                            |
//+------------------------------------------------------------------+
string ProcessTradeCommand(string json)
{
   string action = ExtractJsonValue(json, "action");

   if(action == "RATES")
      return CreateRatesResponse(json);
   if(action == "ACCOUNT_INFO")
      return CreateAccountInfoResponse();

   string symbol = ExtractJsonValue(json, "symbol");
   double volume = StringToDouble(ExtractJsonValue(json, "volume"));
   double sl     = StringToDouble(ExtractJsonValue(json, "sl"));
   double tp     = StringToDouble(ExtractJsonValue(json, "tp"));
   ulong  ticket = (ulong)StringToInteger(ExtractJsonValue(json, "ticket"));

   if(action == "BUY" || action == "SELL")
   {
      if(symbol == "" || volume <= 0.0)
         return CreateJsonResponse("FAILED", 0, "Invalid Symbol or Volume", 0.0);

      SymbolSelect(symbol, true);
      bool success = false;
      double price = 0.0;

      if(action == "BUY")
      {
         price = SymbolInfoDouble(symbol, SYMBOL_ASK);
         success = g_trade.Buy(volume, symbol, price, sl, tp, "AlphaGo Go Engine");
      }
      else if(action == "SELL")
      {
         price = SymbolInfoDouble(symbol, SYMBOL_BID);
         success = g_trade.Sell(volume, symbol, price, sl, tp, "AlphaGo Go Engine");
      }

      if(success)
         return CreateJsonResponse("SUCCESS", g_trade.ResultOrder(), "Order executed successfully", g_trade.ResultPrice());
      else
         return CreateJsonResponse("FAILED", 0, StringFormat("Trade failed. RetCode: %d", g_trade.ResultRetcode()), 0.0);
   }
   else if(action == "CLOSE")
   {
      if(ticket == 0) return CreateJsonResponse("FAILED", 0, "Ticket ID required for CLOSE", 0.0);

      if(g_trade.PositionClose(ticket))
         return CreateJsonResponse("SUCCESS", ticket, "Position closed successfully", g_trade.ResultPrice());
      else
         return CreateJsonResponse("FAILED", ticket, StringFormat("Close failed. RetCode: %d", g_trade.ResultRetcode()), 0.0);
   }
   else if(action == "MODIFY")
   {
      if(ticket == 0) return CreateJsonResponse("FAILED", 0, "Ticket ID required for MODIFY", 0.0);

      if(g_trade.PositionModify(ticket, sl, tp))
         return CreateJsonResponse("SUCCESS", ticket, "Position modified successfully", 0.0);
      else
         return CreateJsonResponse("FAILED", ticket, StringFormat("Modify failed. RetCode: %d", g_trade.ResultRetcode()), 0.0);
   }

   return CreateJsonResponse("FAILED", 0, "Unsupported Action: " + action, 0.0);
}

//+------------------------------------------------------------------+
//| RATES — ขอแท่งเทียนย้อนหลังจากประวัติใน MT5                      |
//|                                                                  |
//| Request (ช่วงเวลา):                                              |
//|   {"action":"RATES","symbol":"XAUUSD","tf":1,"from":<unix>,"to":<unix>} |
//| Request (N แท่งล่าสุด — ไว้ทดสอบง่ายๆ):                          |
//|   {"action":"RATES","symbol":"XAUUSD","tf":1,"count":5}          |
//|                                                                  |
//|   tf   = นาที: 1,5,15,30,60,240,1440                             |
//|   from/to = วินาทีแบบ unix ของ "เวลา server" (เหมือน datetime ใน MQL5) |
//|             ได้แท่งที่ open time อยู่ในช่วง [from, to]           |
//|                                                                  |
//| Response (1 บรรทัด):                                             |
//|   {"status":"SUCCESS","symbol":"XAUUSD","tf":1,"server_time":<unix>, |
//|    "count":2,"has_more":true,"next_from":<unix>,                 |
//|    "rates":[[time,open,high,low,close,tick_volume],...]}         |
//|                                                                  |
//|   rates เรียงจากเก่าไปใหม่ สูงสุด InpMaxRatesPerReply แท่ง       |
//|   has_more=true → ขอต่อด้วย from = next_from                     |
//|   แท่งสุดท้ายอาจเป็นแท่งที่ยังไม่ปิด — Gateway ตัดเองด้วย to      |
//+------------------------------------------------------------------+
string CreateRatesResponse(string json)
{
   string   symbol = ExtractJsonValue(json, "symbol");
   int      tf_min = (int)StringToInteger(ExtractJsonValue(json, "tf"));
   datetime from   = (datetime)StringToInteger(ExtractJsonValue(json, "from"));
   datetime to     = (datetime)StringToInteger(ExtractJsonValue(json, "to"));
   int      count  = (int)StringToInteger(ExtractJsonValue(json, "count"));

   if(symbol == "")
      return CreateJsonResponse("FAILED", 0, "symbol required", 0.0);

   ENUM_TIMEFRAMES period;
   if(!MinutesToTimeframe(tf_min, period))
      return CreateJsonResponse("FAILED", 0, StringFormat("Unsupported tf: %d", tf_min), 0.0);

   if(!SymbolSelect(symbol, true))
      return CreateJsonResponse("FAILED", 0, "Unknown symbol: " + symbol, 0.0);

   MqlRates rates[];
   int copied;
   ResetLastError();

   if(from > 0)
   {
      if(to <= 0) to = TimeCurrent();
      if(from > to)
         return CreateJsonResponse("FAILED", 0, "from must be <= to", 0.0);
      copied = CopyRates(symbol, period, from, to, rates);
   }
   else
   {
      if(count <= 0) count = 1;
      if(count > InpMaxRatesPerReply) count = InpMaxRatesPerReply;
      copied = CopyRates(symbol, period, 0, count, rates);
   }

   // -1 มักแปลว่า MT5 กำลังโหลดประวัติจาก server อยู่ → ให้ฝั่ง Go รอแล้วขอใหม่
   if(copied < 0)
      return CreateJsonResponse("FAILED", 0, StringFormat("CopyRates failed. Error: %d", GetLastError()), 0.0);

   int  n         = (copied > InpMaxRatesPerReply) ? InpMaxRatesPerReply : copied;
   bool has_more  = (copied > n);
   long next_from = has_more ? (long)rates[n].time : 0;
   int  digits    = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);

   string out = StringFormat(
      "{\"status\":\"SUCCESS\",\"symbol\":\"%s\",\"tf\":%d,\"server_time\":%I64d,\"count\":%d,\"has_more\":%s,\"next_from\":%I64d,\"rates\":[",
      symbol, tf_min, (long)TimeCurrent(), n, has_more ? "true" : "false", next_from
   );

   for(int i = 0; i < n; i++)
   {
      if(i > 0) out += ",";
      out += StringFormat("[%I64d,%s,%s,%s,%s,%I64d]",
                          (long)rates[i].time,
                          DoubleToString(rates[i].open,  digits),
                          DoubleToString(rates[i].high,  digits),
                          DoubleToString(rates[i].low,   digits),
                          DoubleToString(rates[i].close, digits),
                          (long)rates[i].tick_volume);
   }
   out += "]}\n";
   return out;
}

bool MinutesToTimeframe(int minutes, ENUM_TIMEFRAMES &period)
{
   switch(minutes)
   {
      case 1:    period = PERIOD_M1;  return true;
      case 5:    period = PERIOD_M5;  return true;
      case 15:   period = PERIOD_M15; return true;
      case 30:   period = PERIOD_M30; return true;
      case 60:   period = PERIOD_H1;  return true;
      case 240:  period = PERIOD_H4;  return true;
      case 1440: period = PERIOD_D1;  return true;
   }
   return false;
}

//+------------------------------------------------------------------+
//| Account Info — ดึง balance/equity/account type/symbol ที่เทรดได้ |
//| สดจาก MT5 โดยตรง (แทนการอ่าน ACCOUNT_BALANCE จาก app.env ฝั่ง Go) |
//+------------------------------------------------------------------+
string CreateAccountInfoResponse()
{
   double balance  = AccountInfoDouble(ACCOUNT_BALANCE);
   double equity   = AccountInfoDouble(ACCOUNT_EQUITY);
   string currency = AccountInfoString(ACCOUNT_CURRENCY);
   long   leverage = AccountInfoInteger(ACCOUNT_LEVERAGE);
   long   login    = AccountInfoInteger(ACCOUNT_LOGIN);
   string broker   = AccountInfoString(ACCOUNT_COMPANY);

   long trade_mode = AccountInfoInteger(ACCOUNT_TRADE_MODE);
   string account_type = "UNKNOWN";
   if(trade_mode == ACCOUNT_TRADE_MODE_DEMO)        account_type = "DEMO";
   else if(trade_mode == ACCOUNT_TRADE_MODE_REAL)    account_type = "REAL";
   else if(trade_mode == ACCOUNT_TRADE_MODE_CONTEST) account_type = "CONTEST";

   // Symbol ที่อยู่ใน Market Watch ตอนนี้ — คือ symbol ที่ "เทรดได้จริง" ไม่ใช่
   // symbol list ทั้งหมดที่ broker มี (SymbolsTotal(false) จะได้ list ยาวเกินจำเป็น)
   string symbols_json = "[";
   int total = SymbolsTotal(true);
   for(int i = 0; i < total; i++)
   {
      if(i > 0) symbols_json += ",";
      symbols_json += "\"" + SymbolName(i, true) + "\"";
   }
   symbols_json += "]";

   return StringFormat(
      "{\"status\":\"SUCCESS\",\"balance\":%.2f,\"equity\":%.2f,\"currency\":\"%s\",\"leverage\":%I64d,\"account_type\":\"%s\",\"broker\":\"%s\",\"login\":%I64d,\"symbols\":%s}\n",
      balance, equity, currency, leverage, account_type, broker, login, symbols_json
   );
}

//+------------------------------------------------------------------+
//| Helpers                                                          |
//+------------------------------------------------------------------+
string ExtractJsonValue(string json, string key)
{
   string search_key = "\"" + key + "\"";
   int key_pos = StringFind(json, search_key);
   if(key_pos < 0) return "";

   int colon_pos = StringFind(json, ":", key_pos);
   if(colon_pos < 0) return "";

   int start = colon_pos + 1;
   int len = StringLen(json);

   while(start < len && (StringGetCharacter(json, start) == ' ' ||
                         StringGetCharacter(json, start) == '\t' ||
                         StringGetCharacter(json, start) == '"'))
      start++;

   int end = start;
   while(end < len)
   {
      ushort c = StringGetCharacter(json, end);
      if(c == '"' || c == ',' || c == '}' || c == ']') break;
      end++;
   }

   return StringSubstr(json, start, end - start);
}

string CreateJsonResponse(string status, ulong ticket, string message, double price)
{
   return StringFormat("{\"status\":\"%s\",\"ticket\":%I64u,\"message\":\"%s\",\"executed_price\":%.5f}\n",
                        status, ticket, message, price);
}