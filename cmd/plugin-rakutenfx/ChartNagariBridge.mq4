//+------------------------------------------------------------------+
//| ChartNagariBridge.mq4                                            |
//| Read-only: pushes Rakuten MT4 DEMO account and order snapshots   |
//| to plugin-rakutenfx. This EA never opens, modifies or closes     |
//| orders.                                                          |
//+------------------------------------------------------------------+
#property strict
#property description "ChartNagari read-only bridge (demo accounts only). Never trades."

input string BridgeURL    = "http://127.0.0.1:9300/bridge/v1/snapshot"; // also add the origin under Tools > Options > Expert Advisors
input string BridgeToken  = "";  // bridge_token from config.local.yaml
input int    IntervalSec  = 5;
input int    HistoryLimit = 50;  // most recent closed/deleted orders to include

int OnInit()
  {
   if(!IsDemo())
     {
      Print("ChartNagariBridge: demo account required; not starting");
      return(INIT_FAILED);
     }
   if(StringLen(BridgeToken) < 32)
     {
      Print("ChartNagariBridge: BridgeToken must contain at least 32 characters");
      return(INIT_PARAMETERS_INCORRECT);
     }
   EventSetTimer(IntervalSec < 1 ? 1 : IntervalSec);
   return(INIT_SUCCEEDED);
  }

void OnDeinit(const int reason) { EventKillTimer(); }

// Appends the selected order; skips it when the contract size is unavailable.
bool AppendOrder(string &list, string status)
  {
   double units = OrderLots() * MarketInfo(OrderSymbol(), MODE_LOTSIZE);
   if(units < 1)
      return(false);
   if(list != "")
      list += ",";
   list += StringFormat("{\"ticket\":\"%d\",\"symbol\":\"%s\",\"status\":\"%s\",\"units\":\"%s\"}",
                        OrderTicket(), OrderSymbol(), status, DoubleToString(units, 0));
   return(true);
  }

void OnTimer()
  {
   string orders = "";
   for(int i = OrdersTotal() - 1; i >= 0; i--)
     {
      if(OrderSelect(i, SELECT_BY_POS, MODE_TRADES))
         AppendOrder(orders, OrderType() <= OP_SELL ? "filled" : "submitted");
     }
   int added = 0;
   for(int i = OrdersHistoryTotal() - 1; i >= 0 && added < HistoryLimit; i--)
     {
      // Types above OP_SELLSTOP are balance/credit entries, not orders.
      if(OrderSelect(i, SELECT_BY_POS, MODE_HISTORY) && OrderType() <= OP_SELLSTOP &&
         AppendOrder(orders, OrderType() <= OP_SELL ? "filled" : "cancelled"))
         added++;
     }
   string body = StringFormat("{\"login\":\"%d\",\"server\":\"%s\",\"trade_mode\":\"%s\",\"currency\":\"%s\",\"free_margin\":\"%s\",\"orders\":[%s]}",
                              AccountNumber(), AccountServer(), IsDemo() ? "demo" : "live",
                              AccountCurrency(), DoubleToString(AccountFreeMargin(), 2), orders);
   char data[], result[];
   string responseHeaders;
   ArrayResize(data, StringToCharArray(body, data, 0, WHOLE_ARRAY, CP_UTF8) - 1); // drop the NUL terminator
   string headers = "Content-Type: application/json\r\nX-ChartNagari-Bridge-Token: " + BridgeToken + "\r\n";
   int code = WebRequest("POST", BridgeURL, headers, 5000, data, result, responseHeaders);
   if(code != 204)
      PrintFormat("ChartNagariBridge: snapshot not accepted (HTTP %d, error %d)", code, GetLastError());
  }
