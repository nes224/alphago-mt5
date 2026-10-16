//+------------------------------------------------------------------+
//|                                          AlphaGo_TickExport.mq5  |
//|  ดึง tick ย้อนหลังจาก MT5 แล้วเขียนเป็น CSV แยกไฟล์รายวัน        |
//|  รูปแบบเดียวกับที่ Gateway recorder เขียน → replay อ่านได้ทั้งคู่ |
//+------------------------------------------------------------------+
#property copyright "AlphaGo Trading System"
#property link      "https://github.com/nes224/alphago-mt5"
#property version   "1.00"
#property script_show_inputs

//--- Inputs
input string   InpSymbol   = "";      // Symbol (ว่าง = symbol ของชาร์ตนี้)
input int      InpDaysBack = 3;       // ย้อนหลังกี่วัน (ใช้เมื่อ From = 0)
input datetime InpFrom     = 0;       // วันเริ่ม (0 = ใช้ DaysBack แทน)
input datetime InpTo       = 0;       // วันสิ้นสุด (0 = ถึงตอนนี้)
input string   InpFolder   = "alphago_ticks"; // โฟลเดอร์ใต้ MQL5\Files

#define SECONDS_PER_DAY 86400
#define MAX_RETRY       5

//+------------------------------------------------------------------+
void OnStart()
{
   string symbol = (InpSymbol == "") ? _Symbol : InpSymbol;
   if(!SymbolSelect(symbol, true))
   {
      PrintFormat("Symbol %s not found", symbol);
      return;
   }
   int digits = (int)SymbolInfoInteger(symbol, SYMBOL_DIGITS);

   // เวลาทั้งหมดเป็นเวลา server ของ broker (เหมือน EA ใช้ TimeCurrent())
   datetime now       = TimeCurrent();
   datetime to        = (InpTo == 0) ? now : InpTo;
   datetime from      = (InpFrom == 0) ? DayStart(now) - (datetime)(InpDaysBack - 1) * SECONDS_PER_DAY
                                       : DayStart(InpFrom);
   if(from > to)
   {
      Print("From must be before To");
      return;
   }

   string dir = InpFolder + "\\" + symbol;
   FolderCreate(InpFolder);
   FolderCreate(dir);

   PrintFormat("Exporting %s ticks %s → %s into %s\\MQL5\\Files\\%s",
               symbol, TimeToString(from), TimeToString(to),
               TerminalInfoString(TERMINAL_DATA_PATH), dir);

   int  files = 0;
   long total = 0;

   for(datetime day = from; day <= to && !IsStopped(); day += SECONDS_PER_DAY)
   {
      datetime day_end = day + SECONDS_PER_DAY - 1;
      if(day_end > to) day_end = to;

      MqlTick ticks[];
      int n = CopyTicksWithRetry(symbol, ticks,
                                 (ulong)day * 1000,
                                 (ulong)day_end * 1000 + 999);
      if(n < 0)
      {
         PrintFormat("%s: CopyTicksRange failed, error %d — skipped", DayName(day), GetLastError());
         continue;
      }
      if(n == 0)
         continue; // วันหยุด / ตลาดปิด ไม่สร้างไฟล์

      int written = WriteDayFile(dir + "\\" + DayName(day) + ".csv", symbol, digits, ticks, n);
      if(written < 0)
         return; // เปิดไฟล์ไม่ได้ — หยุดเลย

      files++;
      total += written;
      PrintFormat("%s: %d ticks", DayName(day), written);
      Comment(StringFormat("AlphaGo export %s: %s (%d ticks)", symbol, DayName(day), written));
   }

   Comment("");
   PrintFormat("Done. %d file(s), %I64d ticks.", files, total);
}

//+------------------------------------------------------------------+
//| CopyTicksRange อาจคืน -1 ระหว่างที่ MT5 กำลังโหลดประวัติจาก server  |
//| → รอแล้วลองใหม่                                                    |
//+------------------------------------------------------------------+
int CopyTicksWithRetry(string symbol, MqlTick &ticks[], ulong from_msc, ulong to_msc)
{
   for(int i = 0; i < MAX_RETRY; i++)
   {
      ResetLastError();
      int n = CopyTicksRange(symbol, ticks, COPY_TICKS_INFO, from_msc, to_msc);
      if(n >= 0)
         return n;
      Sleep(1000);
   }
   return -1;
}

//+------------------------------------------------------------------+
//| เขียน 1 วันเป็น 1 ไฟล์ (เขียนทับถ้ามีอยู่แล้ว)                       |
//| คืนจำนวนแถวที่เขียน หรือ -1 ถ้าเปิดไฟล์ไม่ได้                      |
//+------------------------------------------------------------------+
int WriteDayFile(string path, string symbol, int digits, MqlTick &ticks[], int n)
{
   int h = FileOpen(path, FILE_WRITE | FILE_TXT | FILE_ANSI);
   if(h == INVALID_HANDLE)
   {
      PrintFormat("Cannot open %s, error %d", path, GetLastError());
      return -1;
   }

   FileWriteString(h, "time,symbol,bid,ask,volume\r\n");

   int written = 0;
   for(int i = 0; i < n; i++)
   {
      if(ticks[i].bid <= 0 || ticks[i].ask <= 0)
         continue;

      datetime sec = (datetime)(ticks[i].time_msc / 1000);
      int      ms  = (int)(ticks[i].time_msc % 1000);

      FileWriteString(h, StringFormat("%s.%03d,%s,%s,%s,%I64d\r\n",
                                      TimeToString(sec, TIME_DATE | TIME_SECONDS), ms,
                                      symbol,
                                      DoubleToString(ticks[i].bid, digits),
                                      DoubleToString(ticks[i].ask, digits),
                                      (long)ticks[i].volume));
      written++;
   }

   FileClose(h);
   return written;
}

//+------------------------------------------------------------------+
datetime DayStart(datetime t)
{
   return t - (t % SECONDS_PER_DAY);
}

string DayName(datetime t)
{
   MqlDateTime d;
   TimeToStruct(t, d);
   return StringFormat("%04d-%02d-%02d", d.year, d.mon, d.day);
}