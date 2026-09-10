package main

import (
	"html/template"
	"io"
)

type pageData struct {
	TimeSlots   []ShopSlot
	Products    []pageProduct
	OnSaleCount int
	TotalCount  int
	UpdatedAt   string
}

type pageProduct struct {
	Product
	CardClass string
	TagClass  string
	Badge     string
	Countdown string
}

// RenderPage renders the shared local and static merchant status page.
func RenderPage(w io.Writer, result CrawlResult) error {
	products := append([]Product(nil), result.Products...)
	sortProducts(products, result.TimeSlots)

	view := pageData{
		TimeSlots:   result.TimeSlots,
		Products:    make([]pageProduct, 0, len(products)),
		OnSaleCount: result.OnSaleCount,
		TotalCount:  result.TotalCount,
		UpdatedAt:   result.UpdatedAt,
	}
	for _, product := range products {
		view.Products = append(view.Products, newPageProduct(product))
	}
	return merchantPageTemplate.Execute(w, view)
}

func newPageProduct(product Product) pageProduct {
	view := pageProduct{Product: product}
	switch {
	case product.IsOnSale:
		view.CardClass = "onsale"
		view.TagClass = "tag-ok"
		view.Badge = "✅ 在售"
		view.Countdown = "⏱ 剩余"
	case product.IsUpcoming:
		view.CardClass = "upcoming"
		view.TagClass = "tag-up"
		view.Badge = "⏳ 未开始"
		view.Countdown = "⏱ 距开始"
	default:
		view.CardClass = "ended"
		view.TagClass = "tag-end"
		view.Badge = "❌ 已结束"
		view.Countdown = "—"
	}
	return view
}

var merchantPageTemplate = template.Must(template.New("merchant-page").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><title>远行商人</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Arial,sans-serif;max-width:800px;margin:40px auto;padding:0 20px}
h1{color:#333;margin-bottom:12px}
.card{background:#f5f5f5;border-radius:8px;padding:16px;margin:12px 0}
.onsale{border-left:4px solid #4caf50}
.ended{border-left:4px solid #f44336}
.upcoming{border-left:4px solid #ff9800}
.tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:13px}
.tag-ok{background:#4caf50;color:#fff}
.tag-end{background:#f44336;color:#fff}
.tag-up{background:#ff9800;color:#fff}
.slot{display:inline-block;background:#e3f2fd;padding:4px 12px;border-radius:4px;margin:4px}
.cd{font-weight:bold}
</style></head>
<body>
<h1>🏪 洛克王国 · 远行商人</h1>
<p id="summary">📅 更新时间: <strong>{{.UpdatedAt}}</strong> &nbsp; 📦 {{.TotalCount}}件 ✅ {{.OnSaleCount}}件在售</p>
<p id="time-slots">🕐 时段:{{range .TimeSlots}}<span class="slot">{{.Label}}</span>{{end}}</p>
<h2 style="margin-top:20px">当前商品</h2>
<div id="product-list">{{range .Products}}<div class="card {{.CardClass}}" data-start-at="{{.StartAt}}" data-end-at="{{.EndAt}}">
  <strong>{{.Name}}</strong> <span class="tag {{.TagClass}}">{{.Badge}}</span>
  <p>💰 {{.Price}} &nbsp; 📦 {{.Limit}} &nbsp; 📂 {{.Category}} <span class="slot">🕐 {{.SlotLabel}}</span></p>
  <p class="countdown"><span class="countdown-label">{{.Countdown}}</span>{{if not .HasEnded}} <span class="cd">计算中…</span>{{end}}</p>
</div>{{end}}</div>
<script>
(function(){
function pad(n){return n<10?'0'+n:''+n}
function duration(seconds){
  return pad(Math.floor(seconds/3600))+':'+pad(Math.floor((seconds%3600)/60))+':'+pad(seconds%60);
}
function setStatus(card,status,label,countdown){
  card.classList.remove('onsale','upcoming','ended');
  card.classList.add(status);
  var badge=card.querySelector('.tag');
  badge.classList.remove('tag-ok','tag-up','tag-end');
  badge.classList.add(status==='onsale'?'tag-ok':status==='upcoming'?'tag-up':'tag-end');
  badge.textContent=label;
  var row=card.querySelector('.countdown');
  row.textContent='';
  if(status==='ended'){row.textContent='—';return}
  var prefix=document.createElement('span');
  prefix.className='countdown-label';
  prefix.textContent=status==='onsale'?'⏱ 剩余':'⏱ 距开始';
  var value=document.createElement('span');
  value.className='cd';
  value.textContent=countdown;
  row.appendChild(prefix);
  row.appendChild(document.createTextNode(' '));
  row.appendChild(value);
}
function updateCard(card,now){
  var start=parseInt(card.getAttribute('data-start-at'),10);
  var end=parseInt(card.getAttribute('data-end-at'),10);
  if(!Number.isFinite(start)||!Number.isFinite(end))return;
  if(now<start){setStatus(card,'upcoming','⏳ 未开始',duration(start-now));return}
  if(now<end){setStatus(card,'onsale','✅ 在售',duration(end-now));return}
  setStatus(card,'ended','❌ 已结束','');
}
function updateCountdowns(){
  var now=Math.floor(Date.now()/1000);
  document.querySelectorAll('[data-start-at][data-end-at]').forEach(function(card){updateCard(card,now)});
}
function stringValue(value){return value===null||value===undefined?'':String(value)}
function buildProductCard(product){
  var card=document.createElement('div');
  card.className='card';
  card.setAttribute('data-start-at',stringValue(product.start_at));
  card.setAttribute('data-end-at',stringValue(product.end_at));
  var name=document.createElement('strong');
  name.textContent=product.name;
  card.appendChild(name);
  card.appendChild(document.createTextNode(' '));
  var badge=document.createElement('span');
  badge.className='tag';
  card.appendChild(badge);
  var details=document.createElement('p');
  details.appendChild(document.createTextNode('💰 '+stringValue(product.price)+'   📦 '+stringValue(product.limit)+'   📂 '+stringValue(product.category)+' '));
  var slot=document.createElement('span');
  slot.className='slot';
  slot.textContent='🕐 '+stringValue(product.slot_label);
  details.appendChild(slot);
  card.appendChild(details);
  var countdown=document.createElement('p');
  countdown.className='countdown';
  card.appendChild(countdown);
  updateCard(card,Math.floor(Date.now()/1000));
  return card;
}
function renderSnapshot(snapshot){
  if(!snapshot||!Array.isArray(snapshot.time_slots)||!Array.isArray(snapshot.products))throw new Error('invalid snapshot');
  if(snapshot.time_slots.some(function(slot){return !slot||typeof slot.label!=='string'})||snapshot.products.some(function(product){return !product||typeof product.name!=='string'}))throw new Error('invalid snapshot values');
  var slots=document.createDocumentFragment();
  slots.appendChild(document.createTextNode('🕐 时段:'));
  snapshot.time_slots.forEach(function(item){
    var slot=document.createElement('span');
    slot.className='slot';
    slot.textContent=item.label;
    slots.appendChild(slot);
  });
  var products=document.createDocumentFragment();
  snapshot.products.forEach(function(product){products.appendChild(buildProductCard(product))});
  document.getElementById('summary').textContent='📅 更新时间: '+stringValue(snapshot.updated_at)+'   📦 '+stringValue(snapshot.total_count)+'件 ✅ '+stringValue(snapshot.on_sale_count)+'件在售';
  document.getElementById('time-slots').replaceChildren(slots);
  document.getElementById('product-list').replaceChildren(products);
  updateCountdowns();
}
function loadSnapshot(path){
  return fetch(path,{cache:'no-store'}).then(function(response){
    if(!response.ok)throw new Error('snapshot request failed');
    return response.json();
  }).then(function(payload){
    if(!payload||payload.code!==200||!payload.data)throw new Error('invalid API envelope');
    return payload.data;
  });
}
updateCountdowns();
setInterval(updateCountdowns,1000);
loadSnapshot('products.json')
  .catch(function(){return loadSnapshot('/api/products')})
  .then(function(snapshot){renderSnapshot(snapshot)})
  .catch(function(){});
})();
</script></body></html>`))
