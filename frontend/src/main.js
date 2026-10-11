import './style.css';
import './app.css';
import './local-speedtest.js';
var ln=(e,t)=>()=>(t||e((t={exports:{}}).exports,t),t.exports);var ia=ln((an,sn)=>{(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const r of document.querySelectorAll('link[rel="modulepreload"]'))a(r);new MutationObserver(r=>{for(const i of r)if(i.type==="childList")for(const o of i.addedNodes)o.tagName==="LINK"&&o.rel==="modulepreload"&&a(o)}).observe(document,{childList:!0,subtree:!0});function n(r){const i={};return r.integrity&&(i.integrity=r.integrity),r.referrerpolicy&&(i.referrerPolicy=r.referrerpolicy),r.crossorigin==="use-credentials"?i.credentials="include":r.crossorigin==="anonymous"?i.credentials="omit":i.credentials="same-origin",i}function a(r){if(r.ep)return;r.ep=!0;const i=n(r);fetch(r.href,i)}})();var at=function(){var e=function(R,I){var x=236,T=17,L=R,k=n[I],d=null,l=0,M=null,v=[],w={},U=function(f,g){l=L*4+17,d=function(p){for(var y=new Array(p),h=0;h<p;h+=1){y[h]=new Array(p);for(var E=0;E<p;E+=1)y[h][E]=null}return y}(l),q(0,0),q(l-7,0),q(0,l-7),X(),V(),le(f,g),L>=7&&oe(f),M==null&&(M=on(L,k,v)),de(M,g)},q=function(f,g){for(var p=-1;p<=7;p+=1)if(!(f+p<=-1||l<=f+p))for(var y=-1;y<=7;y+=1)g+y<=-1||l<=g+y||(0<=p&&p<=6&&(y==0||y==6)||0<=y&&y<=6&&(p==0||p==6)||2<=p&&p<=4&&2<=y&&y<=4?d[f+p][g+y]=!0:d[f+p][g+y]=!1)},K=function(){for(var f=0,g=0,p=0;p<8;p+=1){U(!0,p);var y=r.getLostPoint(w);(p==0||f>y)&&(f=y,g=p)}return g},V=function(){for(var f=8;f<l-8;f+=1)d[f][6]==null&&(d[f][6]=f%2==0);for(var g=8;g<l-8;g+=1)d[6][g]==null&&(d[6][g]=g%2==0)},X=function(){for(var f=r.getPatternPosition(L),g=0;g<f.length;g+=1)for(var p=0;p<f.length;p+=1){var y=f[g],h=f[p];if(d[y][h]==null)for(var E=-2;E<=2;E+=1)for(var B=-2;B<=2;B+=1)E==-2||E==2||B==-2||B==2||E==0&&B==0?d[y+E][h+B]=!0:d[y+E][h+B]=!1}},oe=function(f){for(var g=r.getBCHTypeNumber(L),p=0;p<18;p+=1){var y=!f&&(g>>p&1)==1;d[Math.floor(p/3)][p%3+l-8-3]=y}for(var p=0;p<18;p+=1){var y=!f&&(g>>p&1)==1;d[p%3+l-8-3][Math.floor(p/3)]=y}},le=function(f,g){for(var p=k<<3|g,y=r.getBCHTypeInfo(p),h=0;h<15;h+=1){var E=!f&&(y>>h&1)==1;h<6?d[h][8]=E:h<8?d[h+1][8]=E:d[l-15+h][8]=E}for(var h=0;h<15;h+=1){var E=!f&&(y>>h&1)==1;h<8?d[8][l-h-1]=E:h<9?d[8][15-h-1+1]=E:d[8][15-h-1]=E}d[l-8][8]=!f},de=function(f,g){for(var p=-1,y=l-1,h=7,E=0,B=r.getMaskFunction(g),A=l-1;A>0;A-=2)for(A==6&&(A-=1);;){for(var F=0;F<2;F+=1)if(d[y][A-F]==null){var Q=!1;E<f.length&&(Q=(f[E]>>>h&1)==1);var D=B(y,A-F);D&&(Q=!Q),d[y][A-F]=Q,h-=1,h==-1&&(E+=1,h=7)}if(y+=p,y<0||l<=y){y-=p,p=-p;break}}},ve=function(f,g){for(var p=0,y=0,h=0,E=new Array(g.length),B=new Array(g.length),A=0;A<g.length;A+=1){var F=g[A].dataCount,Q=g[A].totalCount-F;y=Math.max(y,F),h=Math.max(h,Q),E[A]=new Array(F);for(var D=0;D<E[A].length;D+=1)E[A][D]=255&f.getBuffer()[D+p];p+=F;var re=r.getErrorCorrectPolynomial(Q),ae=o(E[A],re.getLength()-1),ut=ae.mod(re);B[A]=new Array(re.getLength()-1);for(var D=0;D<B[A].length;D+=1){var Lt=D+ut.getLength()-B[A].length;B[A][D]=Lt>=0?ut.getAt(Lt):0}}for(var ft=0,D=0;D<g.length;D+=1)ft+=g[D].totalCount;for(var je=new Array(ft),Re=0,D=0;D<y;D+=1)for(var A=0;A<g.length;A+=1)D<E[A].length&&(je[Re]=E[A][D],Re+=1);for(var D=0;D<h;D+=1)for(var A=0;A<g.length;A+=1)D<B[A].length&&(je[Re]=B[A][D],Re+=1);return je},on=function(f,g,p){for(var y=u.getRSBlocks(f,g),h=b(),E=0;E<p.length;E+=1){var B=p[E];h.put(B.getMode(),4),h.put(B.getLength(),r.getLengthInBits(B.getMode(),f)),B.write(h)}for(var A=0,E=0;E<y.length;E+=1)A+=y[E].dataCount;if(h.getLengthInBits()>A*8)throw"code length overflow. ("+h.getLengthInBits()+">"+A*8+")";for(h.getLengthInBits()+4<=A*8&&h.put(0,4);h.getLengthInBits()%8!=0;)h.putBit(!1);for(;!(h.getLengthInBits()>=A*8||(h.put(x,8),h.getLengthInBits()>=A*8));)h.put(T,8);return ve(h,y)};w.addData=function(f,g){g=g||"Byte";var p=null;switch(g){case"Numeric":p=S(f);break;case"Alphanumeric":p=C(f);break;case"Byte":p=N(f);break;case"Kanji":p=O(f);break;default:throw"mode:"+g}v.push(p),M=null},w.isDark=function(f,g){if(f<0||l<=f||g<0||l<=g)throw f+","+g;return d[f][g]},w.getModuleCount=function(){return l},w.make=function(){if(L<1){for(var f=1;f<40;f++){for(var g=u.getRSBlocks(f,k),p=b(),y=0;y<v.length;y++){var h=v[y];p.put(h.getMode(),4),p.put(h.getLength(),r.getLengthInBits(h.getMode(),f)),h.write(p)}for(var E=0,y=0;y<g.length;y++)E+=g[y].dataCount;if(p.getLengthInBits()<=E*8)break}L=f}U(!1,K())},w.createTableTag=function(f,g){f=f||2,g=typeof g>"u"?f*4:g;var p="";p+='<table style="',p+=" border-width: 0px; border-style: none;",p+=" border-collapse: collapse;",p+=" padding: 0px; margin: "+g+"px;",p+='">',p+="<tbody>";for(var y=0;y<w.getModuleCount();y+=1){p+="<tr>";for(var h=0;h<w.getModuleCount();h+=1)p+='<td style="',p+=" border-width: 0px; border-style: none;",p+=" border-collapse: collapse;",p+=" padding: 0px; margin: 0px;",p+=" width: "+f+"px;",p+=" height: "+f+"px;",p+=" background-color: ",p+=w.isDark(y,h)?"#000000":"#ffffff",p+=";",p+='"/>';p+="</tr>"}return p+="</tbody>",p+="</table>",p},w.createSvgTag=function(f,g,p,y){var h={};typeof arguments[0]=="object"&&(h=arguments[0],f=h.cellSize,g=h.margin,p=h.alt,y=h.title),f=f||2,g=typeof g>"u"?f*4:g,p=typeof p=="string"?{text:p}:p||{},p.text=p.text||null,p.id=p.text?p.id||"qrcode-description":null,y=typeof y=="string"?{text:y}:y||{},y.text=y.text||null,y.id=y.text?y.id||"qrcode-title":null;var E=w.getModuleCount()*f+g*2,B,A,F,Q,D="",re;for(re="l"+f+",0 0,"+f+" -"+f+",0 0,-"+f+"z ",D+='<svg version="1.1" xmlns="http://www.w3.org/2000/svg"',D+=h.scalable?"":' width="'+E+'px" height="'+E+'px"',D+=' viewBox="0 0 '+E+" "+E+'" ',D+=' preserveAspectRatio="xMinYMin meet"',D+=y.text||p.text?' role="img" aria-labelledby="'+be([y.id,p.id].join(" ").trim())+'"':"",D+=">",D+=y.text?'<title id="'+be(y.id)+'">'+be(y.text)+"</title>":"",D+=p.text?'<description id="'+be(p.id)+'">'+be(p.text)+"</description>":"",D+='<rect width="100%" height="100%" fill="white" cx="0" cy="0"/>',D+='<path d="',F=0;F<w.getModuleCount();F+=1)for(Q=F*f+g,B=0;B<w.getModuleCount();B+=1)w.isDark(F,B)&&(A=B*f+g,D+="M"+A+","+Q+re);return D+='" stroke="transparent" fill="black"/>',D+="</svg>",D},w.createDataURL=function(f,g){f=f||2,g=typeof g>"u"?f*4:g;var p=w.getModuleCount()*f+g*2,y=g,h=p-g;return Ee(p,p,function(E,B){if(y<=E&&E<h&&y<=B&&B<h){var A=Math.floor((E-y)/f),F=Math.floor((B-y)/f);return w.isDark(F,A)?0:1}else return 1})},w.createImgTag=function(f,g,p){f=f||2,g=typeof g>"u"?f*4:g;var y=w.getModuleCount()*f+g*2,h="";return h+="<img",h+=' src="',h+=w.createDataURL(f,g),h+='"',h+=' width="',h+=y,h+='"',h+=' height="',h+=y,h+='"',p&&(h+=' alt="',h+=be(p),h+='"'),h+="/>",h};var be=function(f){for(var g="",p=0;p<f.length;p+=1){var y=f.charAt(p);switch(y){case"<":g+="&lt;";break;case">":g+="&gt;";break;case"&":g+="&amp;";break;case'"':g+="&quot;";break;default:g+=y;break}}return g},cn=function(f){var g=1;f=typeof f>"u"?g*2:f;var p=w.getModuleCount()*g+f*2,y=f,h=p-f,E,B,A,F,Q,D={"\u2588\u2588":"\u2588","\u2588 ":"\u2580"," \u2588":"\u2584","  ":" "},re={"\u2588\u2588":"\u2580","\u2588 ":"\u2580"," \u2588":" ","  ":" "},ae="";for(E=0;E<p;E+=2){for(A=Math.floor((E-y)/g),F=Math.floor((E+1-y)/g),B=0;B<p;B+=1)Q="\u2588",y<=B&&B<h&&y<=E&&E<h&&w.isDark(A,Math.floor((B-y)/g))&&(Q=" "),y<=B&&B<h&&y<=E+1&&E+1<h&&w.isDark(F,Math.floor((B-y)/g))?Q+=" ":Q+="\u2588",ae+=f<1&&E+1>=h?re[Q]:D[Q];ae+=`
`}return p%2&&f>0?ae.substring(0,ae.length-p-1)+Array(p+1).join("\u2580"):ae.substring(0,ae.length-1)};return w.createASCII=function(f,g){if(f=f||1,f<2)return cn(g);f-=1,g=typeof g>"u"?f*2:g;var p=w.getModuleCount()*f+g*2,y=g,h=p-g,E,B,A,F,Q=Array(f+1).join("\u2588\u2588"),D=Array(f+1).join("  "),re="",ae="";for(E=0;E<p;E+=1){for(A=Math.floor((E-y)/f),ae="",B=0;B<p;B+=1)F=1,y<=B&&B<h&&y<=E&&E<h&&w.isDark(A,Math.floor((B-y)/f))&&(F=0),ae+=F?Q:D;for(A=0;A<f;A+=1)re+=ae+`
`}return re.substring(0,re.length-1)},w.renderTo2dContext=function(f,g){g=g||2;for(var p=w.getModuleCount(),y=0;y<p;y++)for(var h=0;h<p;h++)f.fillStyle=w.isDark(y,h)?"black":"white",f.fillRect(y*g,h*g,g,g)},w};e.stringToBytesFuncs={default:function(R){for(var I=[],x=0;x<R.length;x+=1){var T=R.charCodeAt(x);I.push(T&255)}return I}},e.stringToBytes=e.stringToBytesFuncs.default,e.createStringToBytes=function(R,I){var x=function(){for(var L=he(R),k=function(){var V=L.read();if(V==-1)throw"eof";return V},d=0,l={};;){var M=L.read();if(M==-1)break;var v=k(),w=k(),U=k(),q=String.fromCharCode(M<<8|v),K=w<<8|U;l[q]=K,d+=1}if(d!=I)throw d+" != "+I;return l}(),T="?".charCodeAt(0);return function(L){for(var k=[],d=0;d<L.length;d+=1){var l=L.charCodeAt(d);if(l<128)k.push(l);else{var M=x[L.charAt(d)];typeof M=="number"?(M&255)==M?k.push(M):(k.push(M>>>8),k.push(M&255)):k.push(T)}}return k}};var t={MODE_NUMBER:1<<0,MODE_ALPHA_NUM:1<<1,MODE_8BIT_BYTE:1<<2,MODE_KANJI:1<<3},n={L:1,M:0,Q:3,H:2},a={PATTERN000:0,PATTERN001:1,PATTERN010:2,PATTERN011:3,PATTERN100:4,PATTERN101:5,PATTERN110:6,PATTERN111:7},r=function(){var R=[[],[6,18],[6,22],[6,26],[6,30],[6,34],[6,22,38],[6,24,42],[6,26,46],[6,28,50],[6,30,54],[6,32,58],[6,34,62],[6,26,46,66],[6,26,48,70],[6,26,50,74],[6,30,54,78],[6,30,56,82],[6,30,58,86],[6,34,62,90],[6,28,50,72,94],[6,26,50,74,98],[6,30,54,78,102],[6,28,54,80,106],[6,32,58,84,110],[6,30,58,86,114],[6,34,62,90,118],[6,26,50,74,98,122],[6,30,54,78,102,126],[6,26,52,78,104,130],[6,30,56,82,108,134],[6,34,60,86,112,138],[6,30,58,86,114,142],[6,34,62,90,118,146],[6,30,54,78,102,126,150],[6,24,50,76,102,128,154],[6,28,54,80,106,132,158],[6,32,58,84,110,136,162],[6,26,54,82,110,138,166],[6,30,58,86,114,142,170]],I=1<<10|1<<8|1<<5|1<<4|1<<2|1<<1|1<<0,x=1<<12|1<<11|1<<10|1<<9|1<<8|1<<5|1<<2|1<<0,T=1<<14|1<<12|1<<10|1<<4|1<<1,L={},k=function(d){for(var l=0;d!=0;)l+=1,d>>>=1;return l};return L.getBCHTypeInfo=function(d){for(var l=d<<10;k(l)-k(I)>=0;)l^=I<<k(l)-k(I);return(d<<10|l)^T},L.getBCHTypeNumber=function(d){for(var l=d<<12;k(l)-k(x)>=0;)l^=x<<k(l)-k(x);return d<<12|l},L.getPatternPosition=function(d){return R[d-1]},L.getMaskFunction=function(d){switch(d){case a.PATTERN000:return function(l,M){return(l+M)%2==0};case a.PATTERN001:return function(l,M){return l%2==0};case a.PATTERN010:return function(l,M){return M%3==0};case a.PATTERN011:return function(l,M){return(l+M)%3==0};case a.PATTERN100:return function(l,M){return(Math.floor(l/2)+Math.floor(M/3))%2==0};case a.PATTERN101:return function(l,M){return l*M%2+l*M%3==0};case a.PATTERN110:return function(l,M){return(l*M%2+l*M%3)%2==0};case a.PATTERN111:return function(l,M){return(l*M%3+(l+M)%2)%2==0};default:throw"bad maskPattern:"+d}},L.getErrorCorrectPolynomial=function(d){for(var l=o([1],0),M=0;M<d;M+=1)l=l.multiply(o([1,i.gexp(M)],0));return l},L.getLengthInBits=function(d,l){if(1<=l&&l<10)switch(d){case t.MODE_NUMBER:return 10;case t.MODE_ALPHA_NUM:return 9;case t.MODE_8BIT_BYTE:return 8;case t.MODE_KANJI:return 8;default:throw"mode:"+d}else if(l<27)switch(d){case t.MODE_NUMBER:return 12;case t.MODE_ALPHA_NUM:return 11;case t.MODE_8BIT_BYTE:return 16;case t.MODE_KANJI:return 10;default:throw"mode:"+d}else if(l<41)switch(d){case t.MODE_NUMBER:return 14;case t.MODE_ALPHA_NUM:return 13;case t.MODE_8BIT_BYTE:return 16;case t.MODE_KANJI:return 12;default:throw"mode:"+d}else throw"type:"+l},L.getLostPoint=function(d){for(var l=d.getModuleCount(),M=0,v=0;v<l;v+=1)for(var w=0;w<l;w+=1){for(var U=0,q=d.isDark(v,w),K=-1;K<=1;K+=1)if(!(v+K<0||l<=v+K))for(var V=-1;V<=1;V+=1)w+V<0||l<=w+V||K==0&&V==0||q==d.isDark(v+K,w+V)&&(U+=1);U>5&&(M+=3+U-5)}for(var v=0;v<l-1;v+=1)for(var w=0;w<l-1;w+=1){var X=0;d.isDark(v,w)&&(X+=1),d.isDark(v+1,w)&&(X+=1),d.isDark(v,w+1)&&(X+=1),d.isDark(v+1,w+1)&&(X+=1),(X==0||X==4)&&(M+=3)}for(var v=0;v<l;v+=1)for(var w=0;w<l-6;w+=1)d.isDark(v,w)&&!d.isDark(v,w+1)&&d.isDark(v,w+2)&&d.isDark(v,w+3)&&d.isDark(v,w+4)&&!d.isDark(v,w+5)&&d.isDark(v,w+6)&&(M+=40);for(var w=0;w<l;w+=1)for(var v=0;v<l-6;v+=1)d.isDark(v,w)&&!d.isDark(v+1,w)&&d.isDark(v+2,w)&&d.isDark(v+3,w)&&d.isDark(v+4,w)&&!d.isDark(v+5,w)&&d.isDark(v+6,w)&&(M+=40);for(var oe=0,w=0;w<l;w+=1)for(var v=0;v<l;v+=1)d.isDark(v,w)&&(oe+=1);var le=Math.abs(100*oe/l/l-50)/5;return M+=le*10,M},L}(),i=function(){for(var R=new Array(256),I=new Array(256),x=0;x<8;x+=1)R[x]=1<<x;for(var x=8;x<256;x+=1)R[x]=R[x-4]^R[x-5]^R[x-6]^R[x-8];for(var x=0;x<255;x+=1)I[R[x]]=x;var T={};return T.glog=function(L){if(L<1)throw"glog("+L+")";return I[L]},T.gexp=function(L){for(;L<0;)L+=255;for(;L>=256;)L-=255;return R[L]},T}();function o(R,I){if(typeof R.length>"u")throw R.length+"/"+I;var x=function(){for(var L=0;L<R.length&&R[L]==0;)L+=1;for(var k=new Array(R.length-L+I),d=0;d<R.length-L;d+=1)k[d]=R[d+L];return k}(),T={};return T.getAt=function(L){return x[L]},T.getLength=function(){return x.length},T.multiply=function(L){for(var k=new Array(T.getLength()+L.getLength()-1),d=0;d<T.getLength();d+=1)for(var l=0;l<L.getLength();l+=1)k[d+l]^=i.gexp(i.glog(T.getAt(d))+i.glog(L.getAt(l)));return o(k,0)},T.mod=function(L){if(T.getLength()-L.getLength()<0)return T;for(var k=i.glog(T.getAt(0))-i.glog(L.getAt(0)),d=new Array(T.getLength()),l=0;l<T.getLength();l+=1)d[l]=T.getAt(l);for(var l=0;l<L.getLength();l+=1)d[l]^=i.gexp(i.glog(L.getAt(l))+k);return o(d,0).mod(L)},T}var u=function(){var R=[[1,26,19],[1,26,16],[1,26,13],[1,26,9],[1,44,34],[1,44,28],[1,44,22],[1,44,16],[1,70,55],[1,70,44],[2,35,17],[2,35,13],[1,100,80],[2,50,32],[2,50,24],[4,25,9],[1,134,108],[2,67,43],[2,33,15,2,34,16],[2,33,11,2,34,12],[2,86,68],[4,43,27],[4,43,19],[4,43,15],[2,98,78],[4,49,31],[2,32,14,4,33,15],[4,39,13,1,40,14],[2,121,97],[2,60,38,2,61,39],[4,40,18,2,41,19],[4,40,14,2,41,15],[2,146,116],[3,58,36,2,59,37],[4,36,16,4,37,17],[4,36,12,4,37,13],[2,86,68,2,87,69],[4,69,43,1,70,44],[6,43,19,2,44,20],[6,43,15,2,44,16],[4,101,81],[1,80,50,4,81,51],[4,50,22,4,51,23],[3,36,12,8,37,13],[2,116,92,2,117,93],[6,58,36,2,59,37],[4,46,20,6,47,21],[7,42,14,4,43,15],[4,133,107],[8,59,37,1,60,38],[8,44,20,4,45,21],[12,33,11,4,34,12],[3,145,115,1,146,116],[4,64,40,5,65,41],[11,36,16,5,37,17],[11,36,12,5,37,13],[5,109,87,1,110,88],[5,65,41,5,66,42],[5,54,24,7,55,25],[11,36,12,7,37,13],[5,122,98,1,123,99],[7,73,45,3,74,46],[15,43,19,2,44,20],[3,45,15,13,46,16],[1,135,107,5,136,108],[10,74,46,1,75,47],[1,50,22,15,51,23],[2,42,14,17,43,15],[5,150,120,1,151,121],[9,69,43,4,70,44],[17,50,22,1,51,23],[2,42,14,19,43,15],[3,141,113,4,142,114],[3,70,44,11,71,45],[17,47,21,4,48,22],[9,39,13,16,40,14],[3,135,107,5,136,108],[3,67,41,13,68,42],[15,54,24,5,55,25],[15,43,15,10,44,16],[4,144,116,4,145,117],[17,68,42],[17,50,22,6,51,23],[19,46,16,6,47,17],[2,139,111,7,140,112],[17,74,46],[7,54,24,16,55,25],[34,37,13],[4,151,121,5,152,122],[4,75,47,14,76,48],[11,54,24,14,55,25],[16,45,15,14,46,16],[6,147,117,4,148,118],[6,73,45,14,74,46],[11,54,24,16,55,25],[30,46,16,2,47,17],[8,132,106,4,133,107],[8,75,47,13,76,48],[7,54,24,22,55,25],[22,45,15,13,46,16],[10,142,114,2,143,115],[19,74,46,4,75,47],[28,50,22,6,51,23],[33,46,16,4,47,17],[8,152,122,4,153,123],[22,73,45,3,74,46],[8,53,23,26,54,24],[12,45,15,28,46,16],[3,147,117,10,148,118],[3,73,45,23,74,46],[4,54,24,31,55,25],[11,45,15,31,46,16],[7,146,116,7,147,117],[21,73,45,7,74,46],[1,53,23,37,54,24],[19,45,15,26,46,16],[5,145,115,10,146,116],[19,75,47,10,76,48],[15,54,24,25,55,25],[23,45,15,25,46,16],[13,145,115,3,146,116],[2,74,46,29,75,47],[42,54,24,1,55,25],[23,45,15,28,46,16],[17,145,115],[10,74,46,23,75,47],[10,54,24,35,55,25],[19,45,15,35,46,16],[17,145,115,1,146,116],[14,74,46,21,75,47],[29,54,24,19,55,25],[11,45,15,46,46,16],[13,145,115,6,146,116],[14,74,46,23,75,47],[44,54,24,7,55,25],[59,46,16,1,47,17],[12,151,121,7,152,122],[12,75,47,26,76,48],[39,54,24,14,55,25],[22,45,15,41,46,16],[6,151,121,14,152,122],[6,75,47,34,76,48],[46,54,24,10,55,25],[2,45,15,64,46,16],[17,152,122,4,153,123],[29,74,46,14,75,47],[49,54,24,10,55,25],[24,45,15,46,46,16],[4,152,122,18,153,123],[13,74,46,32,75,47],[48,54,24,14,55,25],[42,45,15,32,46,16],[20,147,117,4,148,118],[40,75,47,7,76,48],[43,54,24,22,55,25],[10,45,15,67,46,16],[19,148,118,6,149,119],[18,75,47,31,76,48],[34,54,24,34,55,25],[20,45,15,61,46,16]],I=function(L,k){var d={};return d.totalCount=L,d.dataCount=k,d},x={},T=function(L,k){switch(k){case n.L:return R[(L-1)*4+0];case n.M:return R[(L-1)*4+1];case n.Q:return R[(L-1)*4+2];case n.H:return R[(L-1)*4+3];default:return}};return x.getRSBlocks=function(L,k){var d=T(L,k);if(typeof d>"u")throw"bad rs block @ typeNumber:"+L+"/errorCorrectionLevel:"+k;for(var l=d.length/3,M=[],v=0;v<l;v+=1)for(var w=d[v*3+0],U=d[v*3+1],q=d[v*3+2],K=0;K<w;K+=1)M.push(I(U,q));return M},x}(),b=function(){var R=[],I=0,x={};return x.getBuffer=function(){return R},x.getAt=function(T){var L=Math.floor(T/8);return(R[L]>>>7-T%8&1)==1},x.put=function(T,L){for(var k=0;k<L;k+=1)x.putBit((T>>>L-k-1&1)==1)},x.getLengthInBits=function(){return I},x.putBit=function(T){var L=Math.floor(I/8);R.length<=L&&R.push(0),T&&(R[L]|=128>>>I%8),I+=1},x},S=function(R){var I=t.MODE_NUMBER,x=R,T={};T.getMode=function(){return I},T.getLength=function(d){return x.length},T.write=function(d){for(var l=x,M=0;M+2<l.length;)d.put(L(l.substring(M,M+3)),10),M+=3;M<l.length&&(l.length-M==1?d.put(L(l.substring(M,M+1)),4):l.length-M==2&&d.put(L(l.substring(M,M+2)),7))};var L=function(d){for(var l=0,M=0;M<d.length;M+=1)l=l*10+k(d.charAt(M));return l},k=function(d){if("0"<=d&&d<="9")return d.charCodeAt(0)-"0".charCodeAt(0);throw"illegal char :"+d};return T},C=function(R){var I=t.MODE_ALPHA_NUM,x=R,T={};T.getMode=function(){return I},T.getLength=function(k){return x.length},T.write=function(k){for(var d=x,l=0;l+1<d.length;)k.put(L(d.charAt(l))*45+L(d.charAt(l+1)),11),l+=2;l<d.length&&k.put(L(d.charAt(l)),6)};var L=function(k){if("0"<=k&&k<="9")return k.charCodeAt(0)-"0".charCodeAt(0);if("A"<=k&&k<="Z")return k.charCodeAt(0)-"A".charCodeAt(0)+10;switch(k){case" ":return 36;case"$":return 37;case"%":return 38;case"*":return 39;case"+":return 40;case"-":return 41;case".":return 42;case"/":return 43;case":":return 44;default:throw"illegal char :"+k}};return T},N=function(R){var I=t.MODE_8BIT_BYTE,x=e.stringToBytes(R),T={};return T.getMode=function(){return I},T.getLength=function(L){return x.length},T.write=function(L){for(var k=0;k<x.length;k+=1)L.put(x[k],8)},T},O=function(R){var I=t.MODE_KANJI,x=e.stringToBytesFuncs.SJIS;if(!x)throw"sjis not supported.";(function(k,d){var l=x(k);if(l.length!=2||(l[0]<<8|l[1])!=d)throw"sjis not supported."})("\u53CB",38726);var T=x(R),L={};return L.getMode=function(){return I},L.getLength=function(k){return~~(T.length/2)},L.write=function(k){for(var d=T,l=0;l+1<d.length;){var M=(255&d[l])<<8|255&d[l+1];if(33088<=M&&M<=40956)M-=33088;else if(57408<=M&&M<=60351)M-=49472;else throw"illegal char at "+(l+1)+"/"+M;M=(M>>>8&255)*192+(M&255),k.put(M,13),l+=2}if(l<d.length)throw"illegal char at "+(l+1)},L},G=function(){var R=[],I={};return I.writeByte=function(x){R.push(x&255)},I.writeShort=function(x){I.writeByte(x),I.writeByte(x>>>8)},I.writeBytes=function(x,T,L){T=T||0,L=L||x.length;for(var k=0;k<L;k+=1)I.writeByte(x[k+T])},I.writeString=function(x){for(var T=0;T<x.length;T+=1)I.writeByte(x.charCodeAt(T))},I.toByteArray=function(){return R},I.toString=function(){var x="";x+="[";for(var T=0;T<R.length;T+=1)T>0&&(x+=","),x+=R[T];return x+="]",x},I},$=function(){var R=0,I=0,x=0,T="",L={},k=function(l){T+=String.fromCharCode(d(l&63))},d=function(l){if(!(l<0)){if(l<26)return 65+l;if(l<52)return 97+(l-26);if(l<62)return 48+(l-52);if(l==62)return 43;if(l==63)return 47}throw"n:"+l};return L.writeByte=function(l){for(R=R<<8|l&255,I+=8,x+=1;I>=6;)k(R>>>I-6),I-=6},L.flush=function(){if(I>0&&(k(R<<6-I),R=0,I=0),x%3!=0)for(var l=3-x%3,M=0;M<l;M+=1)T+="="},L.toString=function(){return T},L},he=function(R){var I=R,x=0,T=0,L=0,k={};k.read=function(){for(;L<8;){if(x>=I.length){if(L==0)return-1;throw"unexpected end of file./"+L}var l=I.charAt(x);if(x+=1,l=="=")return L=0,-1;if(l.match(/^\s$/))continue;T=T<<6|d(l.charCodeAt(0)),L+=6}var M=T>>>L-8&255;return L-=8,M};var d=function(l){if(65<=l&&l<=90)return l-65;if(97<=l&&l<=122)return l-97+26;if(48<=l&&l<=57)return l-48+52;if(l==43)return 62;if(l==47)return 63;throw"c:"+l};return k},me=function(R,I){var x=R,T=I,L=new Array(R*I),k={};k.setPixel=function(v,w,U){L[w*x+v]=U},k.write=function(v){v.writeString("GIF87a"),v.writeShort(x),v.writeShort(T),v.writeByte(128),v.writeByte(0),v.writeByte(0),v.writeByte(0),v.writeByte(0),v.writeByte(0),v.writeByte(255),v.writeByte(255),v.writeByte(255),v.writeString(","),v.writeShort(0),v.writeShort(0),v.writeShort(x),v.writeShort(T),v.writeByte(0);var w=2,U=l(w);v.writeByte(w);for(var q=0;U.length-q>255;)v.writeByte(255),v.writeBytes(U,q,255),q+=255;v.writeByte(U.length-q),v.writeBytes(U,q,U.length-q),v.writeByte(0),v.writeString(";")};var d=function(v){var w=v,U=0,q=0,K={};return K.write=function(V,X){if(V>>>X!=0)throw"length over";for(;U+X>=8;)w.writeByte(255&(V<<U|q)),X-=8-U,V>>>=8-U,q=0,U=0;q=V<<U|q,U=U+X},K.flush=function(){U>0&&w.writeByte(q)},K},l=function(v){for(var w=1<<v,U=(1<<v)+1,q=v+1,K=M(),V=0;V<w;V+=1)K.add(String.fromCharCode(V));K.add(String.fromCharCode(w)),K.add(String.fromCharCode(U));var X=G(),oe=d(X);oe.write(w,q);var le=0,de=String.fromCharCode(L[le]);for(le+=1;le<L.length;){var ve=String.fromCharCode(L[le]);le+=1,K.contains(de+ve)?de=de+ve:(oe.write(K.indexOf(de),q),K.size()<4095&&(K.size()==1<<q&&(q+=1),K.add(de+ve)),de=ve)}return oe.write(K.indexOf(de),q),oe.write(U,q),oe.flush(),X.toByteArray()},M=function(){var v={},w=0,U={};return U.add=function(q){if(U.contains(q))throw"dup key:"+q;v[q]=w,w+=1},U.size=function(){return w},U.indexOf=function(q){return v[q]},U.contains=function(q){return typeof v[q]<"u"},U};return k},Ee=function(R,I,x){for(var T=me(R,I),L=0;L<I;L+=1)for(var k=0;k<R;k+=1)T.setPixel(k,L,x(k,L));var d=G();T.write(d);for(var l=$(),M=d.toByteArray(),v=0;v<M.length;v+=1)l.writeByte(M[v]);return l.flush(),"data:image/gif;base64,"+l};return e}();(function(){at.stringToBytesFuncs["UTF-8"]=function(e){function t(n){for(var a=[],r=0;r<n.length;r++){var i=n.charCodeAt(r);i<128?a.push(i):i<2048?a.push(192|i>>6,128|i&63):i<55296||i>=57344?a.push(224|i>>12,128|i>>6&63,128|i&63):(r++,i=65536+((i&1023)<<10|n.charCodeAt(r)&1023),a.push(240|i>>18,128|i>>12&63,128|i>>6&63,128|i&63))}return a}return t(e)}})();(function(e){typeof define=="function"&&define.amd?define([],e):typeof an=="object"&&(sn.exports=e())})(function(){return at});function dn(){return window.go.main.App.ClipboardText()}function NodeDetailApi(e){return window.go.main.App.NodeDetail(e)}function ShareLinkApi(e){return window.go.main.App.ShareLink(e)}function ShareGroupLinksApi(e){return window.go.main.App.ShareGroupLinks(e)}function pn(e){return window.go.main.App.CreateGroup(e)}function un(e){return window.go.main.App.DeleteGroup(e)}function Ln(e){return window.go.main.App.DeleteNode(e)}function fn(e,t,n=!1){return window.go.main.App.ImportNodesToGroupWithDuplicates?window.go.main.App.ImportNodesToGroupWithDuplicates(e,t,n):window.go.main.App.ImportNodesToGroup?window.go.main.App.ImportNodesToGroup(e,t):window.go.main.App.ImportNodes(e)}function gn(e){return window.go.main.App.ImportSubscription(e)}function UpdateSubscriptionGroupApi(e,t){return window.go.main.App.UpdateSubscriptionGroupWithURL?window.go.main.App.UpdateSubscriptionGroupWithURL(e,t||""):window.go.main.App.UpdateSubscriptionGroup(e)}function ReorderGroupsApi(e){return window.go.main.App.ReorderGroups(e)}function ReorderNodesApi(e,t){return window.go.main.App.ReorderNodes(e,t)}function yn(e){return window.go.main.App.ParseTestNodes(e)}function hn(e){return window.go.main.App.PickAppProxyTarget(e)}function mn(){return window.go.main.App.Restart()}function Pt(e){return window.go.main.App.SaveSettings(e)}function RestartTunAsAdminApi(e){return window.go.main.App.RestartTunAsAdmin(e)}function RestartNetworkToolAsAdminApi(){return window.go.main.App.RestartNetworkToolAsAdmin()}function SaveThemeApi(e){return window.go.main.App.SaveTheme?window.go.main.App.SaveTheme(e):Promise.resolve()}function vn(e){return window.go.main.App.SelectNode(e)}function bn(e){return window.go.main.App.StartIperf3(e)}function xn(e){return window.go.main.App.StartMTRWithOptions?window.go.main.App.StartMTRWithOptions(e):window.go.main.App.StartMTR(e.target||e)}function wn(){return window.go.main.App.State()}function LogsApi(){return window.go.main.App.Logs?window.go.main.App.Logs():wn().then(e=>e.logs||[])}function kn(){return window.go.main.App.StopIperf3()}function Mn(){return window.go.main.App.StopMTR()}function Tn(e,t){return window.go.main.App.TestGroupNodesWithMode?window.go.main.App.TestGroupNodesWithMode(e,t||"tcp"):window.go.main.App.TestGroupNodes(e)}function Sn(e,t){return window.go.main.App.TestNodeWithMode?window.go.main.App.TestNodeWithMode(e,t||"tcp"):window.go.main.App.TestNode(e)}function $n(e,t,n){return window.go.main.App.TestRawNode(e,t,n)}function Pn(){return window.go.main.App.ToolStatus()}function Rn(){return window.go.main.App.UpdateCore()}function UpdateRulesApi(){return window.go.main.App.UpdateRules()}function RunNetworkToolApi(e){return window.go.main.App.RunNetworkTool(e)}function In(e,t,n){return window.runtime.EventsOnMultiple(e,t,n)}function Te(e,t){return In(e,t,-1)}function Le(e){return window.runtime.ClipboardSetText(e)}const s={batchSortKey:"",batchSortAsc:!0,page:"home",selectedMode:"global",activeGroupId:"default",nodeRenderedGroupId:"default",nodeScrollTopByGroup:{},rulePresets:[],testingNodeId:"",testingGroupId:"",deletingGroupMode:!1,nodeSearchQuery:"",testNodes:[],testBusyKey:"",testBusyKeys:new Set,batchTesting:!1,ipBackfilling:!1,ipBackfillRunId:0,testRunId:0,testStopRequested:!1,speedTrafficMB:20,speedDurationSec:3,updatingCore:!1,busyKey:"",busyText:"",connectionQuery:"",connectionType:"all",connectionRule:"all",proxyMapScrollTop:0,uiPaused:!1,lastStatePollAt:0,resumeTimer:0,logQuery:"",logLevel:"all",logAutoScroll:!0,clearedLogIndex:0,testerTab:"batch",mtrRunning:!1,mtrOutput:[],iperfRunning:!1,iperfOutput:[],toolStatus:{},applyingSettings:!1,pendingSettings:null,settingsTimer:0,settingsSaveRunning:!1,settingsSaveQueued:!1,settingsVersion:0,settings:{},fullLogs:null,fullLogsLoading:!1},c=e=>document.getElementById(e);function applyTheme(e){const t=e==="dark"?"dark":"light";document.body.dataset.theme=t,s.settings&&(s.settings.theme=t),s.pendingSettings&&(s.pendingSettings.theme=t);try{localStorage.setItem("santian-theme",t)}catch{}const n=c("themeToggle");n&&(n.textContent=t==="dark"?"浅色":"黑色",n.title=t==="dark"?"切换浅色主题":"切换黑色主题",n.setAttribute("aria-pressed",String(t==="dark")))}function initTheme(){let e="light";try{e=localStorage.getItem("santian-theme")||"light"}catch{}applyTheme(e)}function applyStateTheme(e){let t="";try{t=localStorage.getItem("santian-theme")||""}catch{}(t==="dark"||t==="light")&&document.body.dataset.theme!==t&&applyTheme(t)}function toggleTheme(){const e=document.body.dataset.theme==="dark"?"light":"dark";applyTheme(e)}function An(e,t){var a,r,i;const n=(i=(r=(a=window.go)==null?void 0:a.main)==null?void 0:r.App)==null?void 0:i.RenameGroup;if(!n)throw new Error("\u5206\u7EC4\u6539\u540D\u63A5\u53E3\u672A\u5C31\u7EEA\uFF0C\u8BF7\u91CD\u542F\u5F00\u53D1\u6A21\u5F0F");return n(e,t)}const Et=64,logStateLimit=800,Cn=[20,50,100,200],Bn=[3,5,8];document.querySelector("#app").innerHTML=`
  <main class="app-shell">
    <header class="top-bar">
      <nav class="top-menu" aria-label="主菜单">
        <button class="menu active" data-page="home">代理控制</button>
        <button class="menu" data-page="proxyMap">代理映射</button>
        <button class="menu" data-page="connections">连接管理</button>
        <button class="menu" data-page="logs">日志查看</button>
        <button class="menu" data-page="tester">网络工具</button>
      </nav>
      <div id="statusSummary" class="status-summary" title="当前运行状态">
        <span id="runBadge" class="badge">未运行</span>
        <button id="restartProxy" class="status-restart" type="button">重启</button>
        <span id="statusDetail" class="status-detail">代理未启用</span>
        <button id="themeToggle" class="theme-toggle status-theme-toggle" type="button" title="切换黑色主题">黑色</button>
      </div>
      <div class="top-node-area">
        <div id="trafficMonitor" class="traffic-monitor" title="实时流量：上传 0 B/s / 下载 0 B/s">
          <span id="trafficUpload">上传 0 B/s</span>
          <span class="traffic-separator">/</span>
          <span id="trafficDownload">下载 0 B/s</span>
        </div>
        <div id="currentNode" class="current-node" title="当前节点：未选择">
          <span>当前节点</span>
          <strong>未选择</strong>
        </div>
      </div>
    </header>

    <div id="homePage" class="page direct-domains-hidden">
      <section class="grid">
        <div class="panel settings-panel">
          <div class="panel-head">
            <h2>运行设置</h2>
            <div class="version-tools">
              <span id="coreVersion" class="muted">版本检测中</span>
              <button id="updateCore" class="ghost small core-update" type="button" title="检查并更新内核">更新内核</button>
              <button id="updateApp" class="ghost small app-update" type="button" title="检查并更新客户端软件">更新软件</button>
            </div>
          </div>
          <div id="ruleSummary" class="rule-summary hidden">当前规则：检测中</div>
          <div class="proxy-card-row">
            <label class="option-card switch-card" for="systemProxy">
              <span class="option-icon">S</span>
              <span class="option-text">
                <strong>系统代理</strong>
                <span>设置系统代理</span>
              </span>
              <input id="systemProxy" type="checkbox" />
              <span class="toggle-ui" aria-hidden="true"></span>
            </label>
            <label class="option-card switch-card" for="tunProxy">
              <span class="option-icon">T</span>
              <span class="option-text">
                <strong>TUN 代理</strong>
                <span>虚拟网卡代理</span>
              </span>
              <input id="tunProxy" type="checkbox" />
              <span class="toggle-ui" aria-hidden="true"></span>
            </label>
          </div>
          <div class="mode-row" role="group" aria-label="代理模式">
            <button data-mode="global" class="mode option-card active" type="button">
              <span class="option-icon">G</span>
              <span class="option-text">
                <strong>全局模式</strong>
                <span>全部流量代理</span>
              </span>
              <span class="radio-ui" aria-hidden="true"></span>
            </button>
            <button data-mode="rule" class="mode option-card" type="button">
              <span class="option-icon">R</span>
              <span class="option-text">
                <strong>规则模式</strong>
                <span>规则智能分流</span>
              </span>
              <span class="radio-ui" aria-hidden="true"></span>
            </button>
          </div>
          <label class="field direct-domain-field">
            <span id="directDomainsLabel">自定义直连域名</span>
            <textarea id="directDomains" class="direct-domains" placeholder="每行一个域名，例如 example.com"></textarea>
            <small id="directDomainsError" class="field-error"></small>
          </label>
          <details class="more-settings">
            <summary>更多</summary>
            <div class="more-grid">
              <label class="field">
                <span>本地端口</span>
                <input id="mixedPort" type="number" min="1024" max="65535" value="20808" />
              </label>
              <label class="field">
                <span>规则源</span>
                <select id="ruleSource"></select>
              </label>

              <label class="switch">
                <input id="autoStart" type="checkbox" />
                <span>开机自启动</span>
              </label>
              <label class="switch" title="代理节点故障或链路中断时自动拦截海外访问，防止真实 IP 裸奔泄露；国内应用（微信、网页等）保持 100% 畅通">
                <input id="circuitBreaker" type="checkbox" />
                <span>智能分流熔断（防裸奔）</span>
              </label>
              <div class="app-proxy-section">
                <label class="option-card switch-card app-proxy-switch" for="appProxy">
                  <span class="option-icon">A</span>
                  <span class="option-text">
                    <strong>应用接管（TUN）</strong>
                    <span>指定应用通过虚拟网卡走代理</span>
                  </span>
                  <input id="appProxy" type="checkbox" />
                  <span class="toggle-ui" aria-hidden="true"></span>
                </label>
                <div id="appProxyBody" class="app-proxy-body hidden">
                  <div class="app-proxy-actions">
                    <button id="pickAppProxyFolder" class="ghost" type="button">选择文件夹</button>
                    <button id="pickAppProxyApp" class="ghost" type="button">选择应用</button>
                  </div>
                  <div class="app-proxy-list-wrap">
                    <div class="app-proxy-list-head">
                      <strong>应用接管列表</strong>
                      <span id="appProxyCount" class="muted">0 个</span>
                    </div>
                    <div id="appProxyList" class="app-proxy-list empty">暂无应用或文件夹</div>
                  </div>
                </div>
              </div>
            </div>
          </details>
        </div>

        <div class="panel import-panel">
          <div class="panel-head">
            <h2>导入节点</h2>
            <span class="muted">节点进入当前分组，订阅可自动分组</span>
          </div>
          <textarea id="nodeInput" class="import-input" placeholder="粘贴节点链接或订阅链接。单条 http/https 链接按订阅导入，其余按节点导入。"></textarea>
          <button id="importBtn" class="wide primary">导入</button>
        </div>

        <button id="logPreview" class="panel log-preview" type="button">
          <div id="logPreviewItems" class="log-preview-items empty">暂无日志</div>
        </button>
      </section>

      <section class="node-area">
        <div class="panel group-panel">
          <div class="panel-head">
            <div class="group-title">
              <h2>节点分组</h2>
              <span id="nodeCount" class="muted group-node-count">总 0 个节点</span>
            </div>
            <div class="group-head-actions">
              <label id="autoFailoverControl" class="auto-failover-switch" for="autoFailoverSwitch" title="开启后，当当前节点断网时自动切换至同分组最低延迟节点">
                <span class="auto-failover-label">自动切换</span>
                <input id="autoFailoverSwitch" type="checkbox" />
                <span class="toggle-ui" aria-hidden="true"></span>
              </label>
              <input id="nodeSearchInput" class="node-search-input" type="search" placeholder="搜索节点名称" autocomplete="off" />
              <button id="nodeSearchBtn" class="ghost small node-search-btn" type="button">搜索</button>
              <button id="deleteGroupModeBtn" class="ghost small delete-group-mode-btn" type="button">删除分组</button>
              <button id="addGroupBtn" class="ghost small add-group-btn" type="button">添加分组</button>
              <button id="exportBackupBtn" class="ghost small backup-btn" type="button">导出备份</button>
              <button id="importBackupBtn" class="ghost small backup-btn" type="button">导入备份</button>
            </div>
          </div>
          <div id="groups" class="groups"></div>
        </div>

        <div class="panel node-panel">
          <div class="panel-head">
            <h2>节点列表</h2>
            <div class="node-list-head-actions">
              <button id="exportGroupLinksBtn" class="ghost small export-group-btn" type="button" title="导出当前分组全部节点为 Excel（含二维码）">导出分组</button>
            </div>
          </div>
          <div id="nodes" class="nodes empty">暂无节点</div>
        </div>
      </section>
    </div>

    <div id="proxyMapPage" class="page hidden">
      <section class="page-title-row proxy-map-title-row">
        <div class="proxy-map-title">
          <h1>代理映射</h1>
          <span class="muted">为节点生成本地代理端口（支持 SOCKS5 / HTTP）</span>
        </div>
        <div class="proxy-map-top-actions">
          <button id="copyProxyMapSocks5" class="ghost page-action" type="button" title="批量复制所有映射为 socks5://127.0.0.1:端口">复制 SOCKS5</button>
          <button id="exportProxyMapCsv" class="ghost page-action" type="button" title="导出为指纹浏览器 (AdsPower/比特等) 格式 CSV">导出指纹浏览器 CSV</button>
          <button id="copyProxyMapAll" class="primary page-action" type="button" title="复制所有 127.0.0.1:端口">复制全部</button>
        </div>
      </section>
      <section class="panel proxy-map-settings">
        <label class="option-card switch-card" for="proxyMap">
          <span class="option-icon">M</span>
          <span class="option-text">
            <strong>启用映射</strong>
            <span>启用后为节点分配独立本地端口（支持 SOCKS5 与 HTTP）</span>
          </span>
          <input id="proxyMap" type="checkbox" />
          <span class="toggle-ui" aria-hidden="true"></span>
        </label>
        <label class="field proxy-map-port-field">
          <span>起始端口</span>
          <input id="proxyMapStartPort" type="number" min="1024" max="65535" value="52001" />
        </label>
        <div class="field proxy-map-group-field">
          <span>映射分组</span>
          <div id="proxyMapGroupSelect" class="proxy-map-group-select">
            <button id="proxyMapGroupToggle" class="proxy-map-group-toggle" type="button" aria-expanded="false">
              <span id="proxyMapGroupText">全部分组</span>
              <span class="proxy-map-group-arrow" aria-hidden="true">v</span>
            </button>
            <div id="proxyMapGroupMenu" class="proxy-map-group-menu hidden"></div>
          </div>
        </div>
        <div id="proxyMapSummary" class="proxy-map-summary">未启用</div>
      </section>
      <section class="panel proxy-map-panel">
        <div class="panel-head">
          <h2>对应关系</h2>
          <span id="proxyMapCount" class="muted">0 个映射</span>
        </div>
        <div id="proxyMapList" class="proxy-map-list empty">暂无节点</div>
      </section>
    </div>

    <div id="connectionsPage" class="page hidden">
      <section class="page-title-row">
        <div>
          <h1>连接管理</h1>
          <p class="muted">查看当前代理连接和命中规则</p>
        </div>
        <button id="refreshConnections" class="primary page-action">刷新</button>
      </section>
      <section class="panel page-filter-panel">
        <input id="connectionSearch" class="search-input" placeholder="搜索连接、ID、IP、域名或规则..." />
        <select id="connectionType" class="filter-select">
          <option value="all">网络类型</option>
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
          <option value="error">错误连接</option>
        </select>
        <select id="connectionRule" class="filter-select">
          <option value="all">规则类型</option>
          <option value="proxy">代理</option>
          <option value="direct">直连</option>
          <option value="block">拦截 (Block)</option>
          <option value="final">Final</option>
          <option value="error">错误</option>
        </select>
      </section>
      <section class="panel connection-view-panel">
        <div class="connection-list-header">
          <span class="conn-header-meta">协议 / ID / 时间</span>
          <span class="conn-header-flow">来源 ➜ 目标</span>
          <span class="conn-header-rule">命中规则</span>
          <span class="conn-header-status">状态</span>
        </div>
        <div id="connections" class="connection-grid empty">暂无连接</div>
      </section>
    </div>

    <div id="logsPage" class="page hidden">
      <section class="page-title-row">
        <div>
          <h1>日志查看</h1>
          <p class="muted">实时监控系统日志</p>
        </div>
        <div class="log-toolbar">
          <span id="logTotalBadge" class="soft-badge">0 条记录</span>
          <span id="logErrorBadge" class="soft-badge danger">错误: 0</span>
          <label class="log-toggle">
            <input id="logAutoScroll" type="checkbox" checked />
            <span>自动滚动</span>
          </label>
          <button id="clearLogView" class="ghost icon-button" title="清空当前查看">清空</button>
          <button id="copyLogs" class="ghost icon-button" title="复制当前日志">复制</button>
          <button id="exportLogs" class="ghost icon-button" title="导出当前日志">导出</button>
        </div>
      </section>
      <section class="panel log-filter-panel">
        <input id="logSearch" class="search-input" placeholder="搜索日志..." />
        <select id="logLevel" class="filter-select">
          <option value="all">日志级别</option>
          <option value="INFO">信息</option>
          <option value="ERROR">错误</option>
        </select>
      </section>
      <section class="panel log-view-panel">
        <div id="logItems" class="log-items"></div>
        <div class="log-scroll-buttons">
          <button id="logToTop" title="回到顶部">↑</button>
          <button id="logToBottom" title="回到底部">↓</button>
        </div>
      </section>
    </div>

    <div id="testerPage" class="page hidden">
      <div class="tester-tabs" role="tablist" aria-label="节点测试子功能">
        <button class="tester-tab active" data-tester-tab="batch" type="button">批量测试</button>
        <button class="tester-tab" data-tester-tab="mtr" type="button">MTR</button>
        <button class="tester-tab" data-tester-tab="iperf3" type="button">iperf3</button>
        <button id="localSpeedtestTabButton" class="tester-tab" data-tester-tab="localSpeedtest" type="button">本地测速</button>
        <button id="toolboxTabButton" class="tester-tab" data-tester-tab="toolbox" type="button">工具箱</button>
      </div>

      <div id="batchTesterTab" class="tester-tab-page">
        <section class="panel tester-panel">
          <div class="panel-head">
            <h2>批量测试</h2>
            <span class="muted">批量输入节点，每行一个</span>
          </div>
          <textarea id="testInput" class="tester-input" placeholder="粘贴节点链接，或 ip:port:user:pass，每行一个。点击批量测试后自动解析并测 Ping 延迟、URL 延迟和出口 IP。"></textarea>
          <div class="speed-options" aria-label="测参">
            <label class="speed-option">
              <span>测速流量</span>
              <select id="speedTrafficMB">
                <option value="20" selected>20M</option>
                <option value="50">50M</option>
                <option value="100">100M</option>
                <option value="200">200M</option>
              </select>
            </label>
            <label class="speed-option">
              <span>测试时间</span>
              <select id="speedDurationSec">
                <option value="3" selected>3s</option>
                <option value="5">5s</option>
                <option value="8">8s</option>
              </select>
            </label>
            <label class="speed-option">
              <span>测试网络</span>
              <select id="batchNetworkMode">
                <option value="local" selected>本地测试</option>
                <option value="proxy">代理测试</option>
              </select>
            </label>
          </div>
          <div class="tester-actions">
            <button id="pasteTestNodes" class="ghost">剪贴板粘贴</button>
            <button id="batchDelayTest" class="primary">批量测试</button>
            <button id="stopBatchTest" class="ghost danger-action">停止测试</button>
            <button id="filterDeadNodes" class="ghost" type="button" title="剔除所有测试失败或超时的无效节点">剔除失效</button>
            <button id="copyWorkingNodes" class="ghost" type="button" title="复制所有测试可用节点的原始链接">复制可用</button>
            <button id="saveBatchToGroup" class="ghost" type="button" title="将可用节点一键保存为新的独立节点分组">存为新分组</button>
            <button id="clearTestNodes" class="ghost">清空列表</button>
          </div>
        </section>

        <section class="panel test-result-panel">
          <div class="panel-head">
            <h2>测试结果</h2>
            <span id="testNodeCount" class="muted">0 个节点</span>
          </div>
          <div id="testNodes" class="test-nodes empty">暂无测试节点</div>
        </section>
      </div>

      <div id="mtrTesterTab" class="tester-tab-page hidden">
        <section class="panel mtr-panel">
          <div class="panel-head">
            <h2>MTR</h2>
            <span id="mtrStatus" class="muted">NextTrace 检测中</span>
          </div>
          <div class="mtr-form">
            <input id="mtrTarget" class="search-input" placeholder="输入域名或 IP，如 1.1.1.1" />
            <select id="mtrProtocol" class="filter-select">
              <option value="icmp" selected>ICMP (Ping追踪)</option>
              <option value="tcp_443">TCP 443 (HTTPS)</option>
              <option value="tcp_80">TCP 80 (HTTP)</option>
            </select>
            <select id="mtrNetworkMode" class="filter-select">
              <option value="local" selected>本地测试</option>
              <option value="proxy">代理测试</option>
            </select>
            <button id="startMTR" class="primary" type="button">开始</button>
            <button id="stopMTR" class="ghost danger-action" type="button">停止</button>
          </div>
        </section>
        <section class="panel mtr-output-panel">
          <div class="panel-head">
            <h2>MTR 输出</h2>
            <button id="clearMTROutput" class="ghost small" type="button">清空</button>
          </div>
          <pre id="mtrOutput" class="mtr-output">暂无输出</pre>
        </section>
      </div>

      <div id="iperfTesterTab" class="tester-tab-page hidden">
        <section class="panel iperf-panel">
          <div class="panel-head">
            <h2>iperf3</h2>
            <span id="iperfStatus" class="muted">iperf3 检测中</span>
          </div>
          <div class="iperf-form">
            <label class="field">
              <span>目标地址</span>
              <input id="iperfTarget" placeholder="输入 iperf3 服务端地址，如 1.2.3.4 或 1.2.3.4:5201" />
            </label>
            <label class="field">
              <span>网络模式</span>
              <select id="iperfNetworkMode" class="filter-select">
                <option value="direct" selected>本机直连</option>
                <option value="proxy">当前网络/TUN</option>
              </select>
            </label>
            <label class="field">
              <span>并发</span>
              <input id="iperfParallel" type="number" min="1" max="64" value="8" />
            </label>
          </div>
          <div class="iperf-actions">
            <button id="startIperfUpload" class="primary" type="button">开始上传</button>
            <button id="startIperfDownload" class="primary" type="button">开始下载</button>
            <button id="stopIperf" class="ghost danger-action" type="button">停止</button>
          </div>
        </section>
        <section class="panel iperf-output-panel">
          <div class="panel-head">
            <h2>输出</h2>
            <button id="clearIperfOutput" class="ghost small" type="button">清空</button>
          </div>
          <pre id="iperfOutput" class="iperf-output">暂无输出</pre>
        </section>
      </div>

      <div id="localSpeedtestTab" class="tester-tab-page hidden local-speedtest-tab">
        <section class="panel tester-panel">
          <div class="panel-head">
            <h2>本地测速</h2>
            <span class="muted">Ookla Speedtest CLI</span>
          </div>
          <div class="local-speedtest-summary">
            <div>
              <strong>直连网络测速</strong>
              <span>默认直连；开启 TUN 后可选当前网络，更接近网页测速。</span>
            </div>
            <div>
              <strong>结果说明</strong>
              <span>测速服务器默认自动选择，也可以点击获取服务器后选择固定节点。</span>
            </div>
          </div>
          <div class="local-speedtest-server-row">
            <button id="loadLocalSpeedtestServers" class="ghost" type="button">获取服务器</button>
            <select id="localSpeedtestNetworkMode" class="filter-select">
              <option value="direct">本机直连</option>
              <option value="proxy">当前网络/TUN</option>
              <option value="http_proxy">HTTP代理</option>
            </select>
            <select id="localSpeedtestServerSelect" class="filter-select">
              <option value="">自动选择测速服务器</option>
            </select>
          </div>
          <div class="iperf-actions">
            <button id="startLocalSpeedtest" class="primary" type="button">开始测速</button>
            <button id="stopLocalSpeedtest" class="ghost danger-action" type="button" disabled>停止</button>
            <button id="clearLocalSpeedtestOutput" class="ghost" type="button">清空</button>
          </div>
        </section>
        <section class="panel local-speedtest-metrics-panel">
          <div class="speedtest-metrics-grid" id="speedtestMetricsGrid">
            <div class="speedtest-metric-card">
              <div class="speedtest-metric-label">下载速度</div>
              <div class="speedtest-metric-val" id="speedtestDownloadVal">-- <span class="unit">Mbps</span></div>
            </div>
            <div class="speedtest-metric-card">
              <div class="speedtest-metric-label">上传速度</div>
              <div class="speedtest-metric-val" id="speedtestUploadVal">-- <span class="unit">Mbps</span></div>
            </div>
            <div class="speedtest-metric-card">
              <div class="speedtest-metric-label">空闲延迟</div>
              <div class="speedtest-metric-val" id="speedtestPingVal">-- <span class="unit">ms</span></div>
            </div>
            <div class="speedtest-metric-card">
              <div class="speedtest-metric-label">测速节点 / 运营商</div>
              <div class="speedtest-metric-val speedtest-metric-text" id="speedtestServerVal">--</div>
            </div>
          </div>
        </section>
        <section class="panel iperf-output-panel">
          <div class="panel-head">
            <h2>输出</h2>
          </div>
          <pre id="localSpeedtestOutput" class="iperf-output">暂无输出</pre>
        </section>
      </div>
      <div id="toolboxTesterTab" class="tester-tab-page hidden">
        <section class="panel toolbox-panel">
          <div class="panel-head">
            <h2>工具箱</h2>
            <span class="muted">常用网络修复工具</span>
          </div>
          <div class="toolbox-grid">
            <button id="flushDnsCacheTool" class="toolbox-action" data-network-tool="flush_dns" type="button">
              <strong>清理电脑 DNS 缓存</strong>
              <span>运行 ipconfig /flushdns</span>
            </button>
            <button id="repairWinsockTool" class="toolbox-action" data-network-tool="repair_winsock" type="button">
              <strong>Winsock/LSP 修复</strong>
              <span>修复网络套接字</span>
            </button>
            <button id="resetTcpIpTool" class="toolbox-action" data-network-tool="reset_tcpip" type="button">
              <strong>TCP/IP 协议栈重置</strong>
              <span>修复 IP 协议异常</span>
            </button>
            <button id="renewDhcpTool" class="toolbox-action" data-network-tool="renew_dhcp" type="button">
              <strong>刷新 DHCP 租约</strong>
              <span>重新获取 IP 地址</span>
            </button>
            <button id="clearProxyTool" class="toolbox-action" data-network-tool="clear_proxy" type="button">
              <strong>清除代理设置</strong>
              <span>关闭系统代理</span>
            </button>
            <button id="networkResetTool" class="toolbox-action" data-network-tool="network_reset" type="button">
              <strong>重置网络</strong>
              <span>打开网络重置</span>
            </button>
            <button id="checkSystemProxyTool" class="toolbox-action" data-network-tool="check_system_proxy" type="button">
              <strong>检查系统代理</strong>
              <span>查看 Windows 代理设置</span>
            </button>
            <button id="checkWinHttpProxyTool" class="toolbox-action" data-network-tool="check_winhttp_proxy" type="button">
              <strong>检查 WinHTTP 代理</strong>
              <span>查看系统服务代理</span>
            </button>
            <button id="checkLocalProxyPortTool" class="toolbox-action" data-network-tool="check_local_proxy_port" type="button">
              <strong>检查本地代理端口</strong>
              <span>确认端口是否监听</span>
            </button>
            <button id="repairProxyResidualTool" class="toolbox-action" data-network-tool="repair_proxy_residual" type="button">
              <strong>清理代理残留</strong>
              <span>清理系统代理和环境变量</span>
            </button>
            <button id="cleanupTunResidualTool" class="toolbox-action" data-network-tool="cleanup_tun_residual" type="button">
              <strong>清理 TUN 残留</strong>
              <span>清理虚拟网卡路由 DNS</span>
            </button>
            <button id="syncTimeTool" class="toolbox-action" data-network-tool="sync_time" type="button">
              <strong>系统时间 NTP 校验与对时</strong>
              <span>修复节点连接/TLS因时间偏差失效</span>
            </button>
            <button id="repairUwpLoopbackTool" class="toolbox-action" data-network-tool="repair_uwp_loopback" type="button">
              <strong>UWP 应用回环代理免除</strong>
              <span>解决微软商店/Xbox/Copilot代理不可用</span>
            </button>
            <button id="killOccupiedPortTool" class="toolbox-action" data-network-tool="kill_occupied_port" type="button">
              <strong>释放被占用的本地代理端口</strong>
              <span>排查并强制释放 20808/52001 端口冲突</span>
            </button>
            <button id="clearTerminalGitProxyTool" class="toolbox-action" data-network-tool="clear_terminal_git_proxy" type="button">
              <strong>清理终端与 Git 代理残留</strong>
              <span>清除 CMD/PowerShell/Git 全局代理配置</span>
            </button>
            <button id="fixFirewallRuleTool" class="toolbox-action" data-network-tool="fix_firewall_rule" type="button">
              <strong>Windows 防火墙放行修复</strong>
              <span>重新放行 sing-box 内核入站与出站通信</span>
            </button>
            <button id="toggleIpv6Tool" class="toolbox-action" data-network-tool="toggle_ipv6" type="button">
              <strong>IPv6 诊断与一键开关</strong>
              <span>排查 IPv6 绕过代理或 DNS 泄露并一键切换</span>
            </button>
          </div>
        </section>
      </div>
    </div>
  </main>
  <div id="confirmDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="confirmTitle">
    <div class="confirm-card">
      <div class="confirm-content">
        <h2 id="confirmTitle">确认操作</h2>
        <p id="confirmMessage"></p>
        <span id="confirmDesc"></span>
      </div>
      <div class="confirm-actions">
        <button id="confirmCancel" class="ghost" type="button">取消</button>
        <button id="confirmOk" class="danger" type="button">确认</button>
      </div>
    </div>
  </div>
  <div id="subscriptionUpdateDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="subscriptionUpdateTitle">
    <div class="confirm-card subscription-update-card">
      <button id="subscriptionUpdateClose" class="circle-close" type="button" aria-label="关闭">×</button>
      <div class="confirm-content subscription-update-content">
        <h2 id="subscriptionUpdateTitle">更新订阅</h2>
        <p id="subscriptionUpdateMessage">是否更新订阅？</p>
        <span>会重新获取订阅内容，并替换该分组里的旧节点。</span>
        <label class="field subscription-url-field">
          <span>订阅链接</span>
          <div class="subscription-url-row">
            <input id="subscriptionUpdateURL" type="url" placeholder="https://example.com/sub" />
            <button id="subscriptionCopyURL" class="ghost" type="button">复制</button>
          </div>
        </label>
      </div>
      <div class="confirm-actions">
        <button id="subscriptionUpdateCancel" class="ghost" type="button">取消</button>
        <button id="subscriptionUpdateOk" class="danger" type="button">更新</button>
      </div>
    </div>
  </div>
  <div id="testModeDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="testModeTitle">
    <div class="confirm-card test-mode-card">
      <div class="confirm-content">
        <h2 id="testModeTitle">测试前需要处理 TUN</h2>
        <p id="testModeMessage">TUN 模式会接管本机流量，建议先关闭 TUN 或切换为系统代理后再使用本地直连测试。</p>
        <span id="testModeDesc">选择后软件会自动应用设置，再开始测试。</span>
      </div>
      <div class="test-mode-actions">
        <button id="testModeDisableTun" class="primary" type="button">关闭 TUN</button>
        <button id="testModeSwitchSystem" class="ghost" type="button">切换系统代理</button>
        <button id="testModeCancel" class="ghost" type="button">取消</button>
      </div>
    </div>
  </div>
  <div id="delayMetricDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="delayMetricTitle">
    <div class="confirm-card delay-metric-card">
      <div class="confirm-content">
        <h2 id="delayMetricTitle">选择测速方式</h2>
        <p id="delayMetricMessage">请选择本次要测试的延迟类型。</p>
        <span>TCP延迟：本地直连节点服务器，测量物理网络延迟（极速，推荐）；<br>真连接延迟：通过节点代理访问外网，测试实际出站往返。</span>
      </div>
      <div class="delay-metric-actions">
        <button id="delayMetricTcp" class="primary" type="button">TCP延迟（推荐）</button>
        <button id="delayMetricReal" class="ghost" type="button">真连接延迟</button>
        <button id="delayMetricCancel" class="ghost" type="button">取消</button>
      </div>
    </div>
  </div>  <div id="updateChoiceDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="updateChoiceTitle">
    <div class="confirm-card update-choice-card">
      <div class="update-choice-head">
        <div>
          <h2 id="updateChoiceTitle">选择更新内容</h2>
          <p>按需更新内核或规则，不改变代理模式</p>
        </div>
        <button id="updateChoiceClose" class="ghost update-choice-close" type="button" aria-label="关闭">×</button>
      </div>
      <div class="update-choice-actions">
        <button id="updateChoiceCore" class="update-choice-option primary" type="button">
          <span>sing-box</span>
          <small>只更新内核</small>
        </button>
        <button id="updateChoiceRules" class="update-choice-option ghost" type="button">
          <span>更新规则</span>
          <small>只更新路由规则</small>
        </button>
        <button id="updateChoiceApp" class="update-choice-option ghost" type="button">
          <span>更新软件</span>
          <small>客户端新版本</small>
        </button>
      </div>
      <div class="update-choice-footer">
        <button id="updateChoiceAll" class="ghost" type="button">全部</button>
        <button id="updateChoiceCancel" class="ghost" type="button">取消</button>
      </div>
    </div>
  </div>
  <div id="appUpdateDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="appUpdateTitle">
    <div class="confirm-card app-update-card">
      <div class="app-update-head">
        <div>
          <span class="app-update-badge">新版本发布</span>
          <h2 id="appUpdateTitle">发现软件新版本</h2>
          <p id="appUpdateVersionText" class="app-update-version">检测中...</p>
        </div>
        <button id="appUpdateClose" class="ghost update-choice-close" type="button" aria-label="关闭">×</button>
      </div>
      <div class="app-update-body">
        <div class="app-update-notes-title">更新说明</div>
        <div id="appUpdateNotes" class="app-update-notes">暂无详细更新说明</div>
        <div id="appUpdateProgressBox" class="app-update-progress-box hidden">
          <div class="app-update-progress-status">
            <span id="appUpdateStatusText">正在准备更新...</span>
            <span id="appUpdatePercentText">0%</span>
          </div>
          <div class="app-update-progress-bar-bg">
            <div id="appUpdateProgressBar" class="app-update-progress-bar" style="width: 0%"></div>
          </div>
        </div>
      </div>
      <div class="confirm-actions app-update-actions">
        <button id="appUpdateCancel" class="ghost" type="button">取消</button>
        <button id="appUpdateConfirm" class="primary" type="button">立即更新</button>
      </div>
    </div>
  </div>
  <div id="groupDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="groupDialogTitle">
    <div class="confirm-card group-dialog-card">
      <div class="confirm-content">
        <h2 id="groupDialogTitle">添加分组</h2>
        <p id="groupDialogMessage">每个组管理不同的节点</p>
        <label class="field group-name-field">
          <span>节点组名称</span>
          <input id="groupNameInput" type="text" maxlength="40" placeholder="输入节点组名称" />
        </label>
      </div>
      <div class="confirm-actions">
        <button id="groupCancel" class="ghost" type="button">取消</button>
        <button id="groupOk" class="primary" type="button">保存</button>
      </div>
    </div>
  </div>
  <div id="nodeLinkDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="nodeLinkTitle">
    <div class="confirm-card node-link-card">
      <div class="confirm-content">
        <h2 id="nodeLinkTitle">节点信息</h2>
        <p id="nodeLinkName"></p>
        <div id="nodeQrBox" class="node-qr-box hidden"></div>
        <textarea id="nodeLinkText" readonly></textarea>
      </div>
      <div class="confirm-actions">
        <button id="nodeLinkClose" class="ghost" type="button">关闭</button>
        <button id="nodeLinkCopy" class="primary" type="button">复制</button>
      </div>
    </div>
  </div>
  <div id="nodeEditDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="nodeEditTitle">
    <div class="confirm-card node-edit-card">
      <div class="confirm-content">
        <div class="node-edit-head">
          <div>
            <h2 id="nodeEditTitle">编辑节点</h2>
            <p id="nodeEditProtocol"></p>
          </div>
          <button id="nodeEditClose" class="node-edit-close" type="button" aria-label="关闭编辑节点">×</button>
        </div>
        <form id="nodeEditForm" class="node-edit-form"></form>
      </div>
      <div class="confirm-actions">
        <button id="nodeEditCancel" class="ghost" type="button">取消</button>
        <button id="nodeEditSave" class="primary" type="button">保存</button>
      </div>
    </div>
  </div>
  <div id="networkToolDialog" class="confirm-dialog hidden" role="dialog" aria-modal="true" aria-labelledby="networkToolTitle">
    <div class="confirm-card network-tool-card">
      <div class="confirm-content">
        <h2 id="networkToolTitle">工具运行结果</h2>
        <p id="networkToolCommand"></p>
        <pre id="networkToolOutput" class="network-tool-output">暂无输出</pre>
      </div>
      <div class="confirm-actions">
        <button id="networkToolClose" class="primary" type="button">关闭</button>
      </div>
    </div>
  </div>  <div class="notice-stack">
    <div id="toast" class="toast" role="status" aria-live="polite"></div>
    <div id="activity" class="activity hidden" role="status" aria-live="polite">
      <span class="spinner" aria-hidden="true"></span>
      <span id="activityText">处理中</span>
    </div>
  </div>
`;

const exclusiveProxyEntryIDs = ["systemProxy", "tunProxy", "appProxy"];
document.addEventListener("change", (event) => {
  const target = event.target;
  if (!target?.checked || !exclusiveProxyEntryIDs.includes(target.id)) return;
  exclusiveProxyEntryIDs.forEach((id) => {
    if (id !== target.id) document.getElementById(id).checked = false;
  });
}, true);

[
  ["flushDnsCacheTool", "flush_dns"],
  ["repairWinsockTool", "repair_winsock"],
  ["resetTcpIpTool", "reset_tcpip"],
  ["renewDhcpTool", "renew_dhcp"],
  ["clearProxyTool", "clear_proxy"],
  ["networkResetTool", "network_reset"],
  ["checkSystemProxyTool", "check_system_proxy"],
  ["checkWinHttpProxyTool", "check_winhttp_proxy"],
  ["checkLocalProxyPortTool", "check_local_proxy_port"],
  ["repairProxyResidualTool", "repair_proxy_residual"],
  ["cleanupTunResidualTool", "cleanup_tun_residual"],
  ["syncTimeTool", "sync_time"],
  ["repairUwpLoopbackTool", "repair_uwp_loopback"],
  ["killOccupiedPortTool", "kill_occupied_port"],
  ["clearTerminalGitProxyTool", "clear_terminal_git_proxy"],
  ["fixFirewallRuleTool", "fix_firewall_rule"],
  ["toggleIpv6Tool", "toggle_ipv6"],
].forEach(([id, tool]) => {
  const button = document.getElementById(id);
  if (button) button.addEventListener("click", () => runNetworkTool(tool));
});
async function Me(){if(s.uiPaused||document.hidden)return;if(window.__sanTianStateLoading){window.__sanTianStateQueued=!0;return}window.__sanTianStateLoading=!0;try{s.lastStatePollAt=Date.now();const e=await wn();s.uiPaused||Ge(e)}finally{window.__sanTianStateLoading=!1,!s.uiPaused&&window.__sanTianStateQueued&&(window.__sanTianStateQueued=!1,setTimeout(()=>Me().catch(()=>{}),350))}}function Ge(e){e=e||{},Array.isArray(e.logs)&&e.logs.length>logStateLimit&&(e={...e,logs:e.logs.slice(-logStateLimit)},s.clearedLogIndex=Math.min(s.clearedLogIndex,e.logs.length));const t=!Rt();s.rulePresets=e.rulePresets||s.rulePresets||[],t?(s.settings=e.settings||{},s.selectedMode=ke(s.settings.mode),applyStateTheme(s.settings.theme)):s.pendingSettings&&(s.settings=s.pendingSettings,s.selectedMode=ke(s.settings.mode)),s.lastState=e,document.body.classList.add("state-ready");const n=Boolean(e.running),a=W("refreshConnections"),r=Boolean(s.busyKey);const cb=Boolean(e.circuitBreakerActive);runBadge.textContent=cb?"安全熔断中":n?"\u8FD0\u884C\u4E2D":"\u672A\u8FD0\u884C",c("runBadge").classList.toggle("running",n&&!cb),c("runBadge").classList.toggle("circuit-breaker",cb),c("runBadge").classList.toggle("working",W("settings")),cb&&!s.lastCircuitBreakerActive&&P("【智能分流熔断】海外代理链路异常，已自动拦截海外流量以防直连泄露，国内网络保持正常畅通。"),s.lastCircuitBreakerActive=cb,Y(updateCore,s.updatingCore||W("updateCore"),"\u66F4\u65B0\u4E2D","\u66F4\u65B0"),c("updateCore").disabled=Boolean(r||s.updatingCore),c("updateCore").classList.toggle("update-available",Boolean(e.coreUpdateAvailable)),c("updateApp")&&c("updateApp").classList.toggle("update-available",Boolean(e.appUpdateAvailable)),c("updateApp")&&(c("updateApp").disabled=Boolean(r)),Y(refreshConnections,a,"\u5237\u65B0\u4E2D","\u5237\u65B0"),c("refreshConnections").disabled=r,c("coreVersion").textContent=`内核 ${e.coreVersion||"内核未知"}`,updateTrafficMonitor(e),On(e.nodes||[],e.selectedId),_n(e.rulePresets||[]),t&&(c("mixedPort").value=s.settings.mixedPort||20808,c("proxyMap").checked=Boolean(s.settings.proxyMap),c("proxyMapStartPort").value=s.settings.proxyMapStartPort||52001,Dn(e.groups||[],Ze(s.settings)),c("systemProxy").checked=Boolean(s.settings.systemProxy),c("tunProxy").checked=Boolean(s.settings.tun),c("ruleSource").value=s.settings.ruleSource||"sagernet",c("autoStart").checked=!1,c("circuitBreaker")&&(c("circuitBreaker").checked=s.settings.circuitBreaker!==!1),c("appProxy").checked=Boolean(s.settings.appProxy),Ke(s.settings.appProxyTargets||[])),(()=>{const af=Boolean(s.settings.autoFailover);const sw=c("autoFailoverSwitch");if(sw)sw.checked=af;const ctrl=c("autoFailoverControl");if(ctrl){ctrl.classList.toggle("active",af),ctrl.title=af?"已开启自动切换：节点断网时将自动切换至同分组最低延迟节点（点击关闭）":"已关闭自动切换：仅手动切换节点（点击开启）"}const b=c("autoFailoverBtn");if(b){b.classList.toggle("active",af),b.textContent=af?"自动切换：开":"自动切换：关",b.title=af?"已开启自动切换：节点断网时将自动切换至同分组最低延迟节点（点击关闭）":"已关闭自动切换：仅手动切换节点（点击开启）"}})(),t&&document.activeElement!==c("directDomains")&&(c("directDomains").value=s.settings.directDomains||""),updateDirectDomainsVisibility(),c("ruleSource").disabled=n,c("autoStart").disabled=!0,c("appProxy").disabled=!1,c("pickAppProxyFolder").disabled=r,c("pickAppProxyApp").disabled=r,_t(),c("mixedPort").disabled=n,c("proxyMap").disabled=Rt(),c("proxyMapStartPort").disabled=n,c("proxyMapGroupToggle").disabled=Rt(),c("systemProxy").disabled=Rt(),c("tunProxy").disabled=Rt(),c("directDomains").disabled=ke(s.selectedMode)!=="rule",document.querySelectorAll(".mode").forEach(o=>{o.classList.toggle("active",o.dataset.mode===s.selectedMode),o.disabled=Rt()||r&&!W("settings")}),te(),refreshVisibleStatePanels(e);const i=t?e.configMode:{...e.configMode||{},mode:s.settings.mode};Zn({...e,settings:s.settings},i)}function refreshVisibleStatePanels(e={}){const t=e.nodes||[],n=e.groups||[],a=e.logs||[];s.page==="home"?(Oe(n,t),Ce(t,e.selectedId)):s.page==="proxyMap"?It(t):s.page==="connections"?Be(a):s.page==="logs"&&loadFullLogs(),Xn(a)}function Rt(){return Boolean(s.applyingSettings||s.pendingSettings||s.settingsSaveRunning)}async function loadFullLogs(){if(s.fullLogsLoading)return;s.fullLogsLoading=!0;try{const e=await LogsApi();s.fullLogs=Array.isArray(e)?e:[],Se(s.fullLogs)}catch(e){P("读取完整日志失败："+J(e))}finally{s.fullLogsLoading=!1}}function _n(e){const t=c("ruleSource"),n=t.value;t.innerHTML=e.map(a=>`<option value="${m(a.id)}">${m(a.name)}</option>`).join(""),n&&(t.value=n)}function Dn(e,t){const n=c("proxyMapGroupMenu"),a=new Set(t||[]);n.innerHTML=(e||[]).map(o=>`
      <label class="proxy-map-group-option">
        <input type="checkbox" data-proxy-map-group="${m(o.id)}" />
        <span>${m(o.name)}</span>
      </label>
    `).join("");const r=new Set((e||[]).map(o=>o.id)),i=Array.from(a).filter(o=>r.has(o));n.querySelectorAll("[data-proxy-map-group]").forEach(o=>{o.checked=i.includes(o.dataset.proxyMapGroup),o.addEventListener("change",()=>{Zr(o),saveProxyMapGroupsOnly()})}),Gt(e,i),s.settings.proxyMapGroupIds=i,s.settings.proxyMapGroupId=i[0]||""}function updateTrafficMonitor(e={}){const t=Number(e.uploadRate||0),n=Number(e.downloadRate||0),a=Ne(t)+"/s",r=Ne(n)+"/s",i=c("trafficMonitor");c("trafficUpload").textContent=`上传 ${a}`,c("trafficDownload").textContent=`下载 ${r}`,i&&(i.title=`实时流量：上传 ${a} / 下载 ${r}`)}function On(e,t){const n=e.find(i=>i.id===t),a=(n==null?void 0:n.name)||"未选择",r=c("currentNode");r.querySelector("strong").textContent=a,r.title="当前节点："+a,r.classList.toggle("empty",!n)}function Zn(e={},t=e.configMode||{}){var o;const n=e.settings||s.settings||{},a=ke(t.mode||n.mode||s.selectedMode),r=Boolean(e.running),i=[];(o=s.rulePresets.find(u=>u.id===n.ruleSource))!=null&&o.name||n.ruleSource,n.systemProxy&&i.push("系统代理"),n.tun&&i.push(e.tunStatus||"TUN代理"),n.proxyMap&&i.push("代理映射"),n.appProxy&&i.push("全局代理应用"),i.push(a==="global"?"全局模式":"规则模式"),c("statusSummary").title=`当前状态：${i.join(" / ")}`,statusSummary.querySelector("#statusDetail").textContent=i.join(" / ")||"\u4EE3\u7406\u672A\u542F\u7528",c("statusSummary").classList.toggle("running",r),c("ruleSummary").textContent="当前规则："+(n.ruleSource||"未选择"),c("ruleSummary").title="当前规则："+(n.ruleSource||"未选择"),c("ruleSummary").classList.remove("update-available")}function Ke(e=[]){var i;const t=c("appProxyBody"),n=c("appProxyList"),a=Boolean((i=c("appProxy"))==null?void 0:i.checked),r=Kt(Array.isArray(e)?e:[]);if(t.classList.toggle("hidden",!a),c("appProxyCount").textContent=`${r.length} 个`,!r.length){n.className="app-proxy-list empty",n.textContent="暂无应用或文件夹",s.settings.appProxyTargets=r;return}s.settings.appProxyTargets=r,n.className="app-proxy-list",n.innerHTML=r.map(o=>`
    <div class="app-proxy-item">
      <div class="app-proxy-main">
        <span class="app-proxy-kind">${o.type==="folder"?"文件夹":"应用"}</span>
        <strong title="${m(o.path)}">${m(o.name||Un(o.path))}</strong>
        <span title="${m(o.path)}">${m(o.path)}</span>
      </div>
      <button class="ghost app-proxy-delete" data-app-proxy-delete="${m(o.id)}" type="button">删除</button>
    </div>
  `).join(""),n.querySelectorAll("[data-app-proxy-delete]").forEach(o=>{o.addEventListener("click",()=>{s.settings.appProxyTargets=(s.settings.appProxyTargets||[]).filter(u=>u.id!==o.dataset.appProxyDelete),Ke(s.settings.appProxyTargets),ue(0)})})}function Un(e){return String(e||"").split(/[\\/]/).filter(Boolean).pop()||"\u672A\u547D\u540D"}function Oe(e,t){var a;e.some(r=>r.id===s.activeGroupId)||(s.activeGroupId=((a=e[0])==null?void 0:a.id)||"default");const n=c("groups"),nodeCountByGroup=new Map,rawCountByGroup=new Map;(t||[]).forEach(r=>{const i=r.groupId||"default";nodeCountByGroup.set(i,(nodeCountByGroup.get(i)||0)+1),rawCountByGroup.set(i,(rawCountByGroup.get(i)||0)+1)});let dragId="",groupClickTimer=0;const refreshActive=()=>{n.querySelectorAll("[data-group-open]").forEach(r=>{r.classList.toggle("active",r.dataset.groupOpen===s.activeGroupId)})};n.innerHTML=e.map(r=>{const i=nodeCountByGroup.get(r.id)||0,o=r.id===s.activeGroupId,u=r.id===s.testingGroupId,b=W(`deleteGroup:${r.id}`),S=W(`copyGroup:${r.id}`),C=Boolean(s.busyKey||s.testingGroupId),O=rawCountByGroup.get(r.id)||0,D=r.id!=="default"&&!C;return`
        <article class="group-card ${o?"active":""} ${s.deletingGroupMode?"delete-pick":""} ${D?"draggable":"fixed"}" data-group-open="${r.id}" draggable="${D?"true":"false"}">
          <div class="group-main">
            <strong title="${m(r.name)}" data-group-rename="${r.id}">${m(r.name)}</strong>
            <span>${i} 个节点</span>
            <span class="traffic-pill">流量：${Ne(zt(r))}</span>
          </div>
          <div class="group-actions">
            <div class="group-buttons with-copy">
              <button class="${S?"is-busy":""}" data-group-copy="${r.id}" title="复制 ${O} 条链接" ${C?"disabled":""}>${S?"复制中":"复制"}</button>
              <button class="${u?"is-busy":""}" data-group-test="${r.id}" ${i===0||C?"disabled":""}>${u?"测试中":"测试"}</button>
              <button class="${b?"is-busy":""}" data-group-delete="${r.id}" ${r.id==="default"||C?"disabled":""}>删除</button>
            </div>
            <span class="rate-pill">上传 ${Ne(r.uploadRate)}/s / 下载 ${Ne(r.downloadRate)}/s</span>
          </div>
        </article>
      `}).join("");const saveOrder=()=>{const r=Array.from(n.querySelectorAll("[data-group-open]")).map(i=>i.dataset.groupOpen).filter(i=>i&&i!=="default");H("reorderGroups","正在保存分组排序",async()=>{await ReorderGroupsApi(r),P("节点组排序已保存")},null,{skipRefresh:!0})};n.querySelectorAll("[data-group-open]").forEach(r=>{r.addEventListener("dragstart",i=>{if(r.dataset.groupOpen==="default"||s.busyKey){i.preventDefault();return}dragId=r.dataset.groupOpen,r.classList.add("dragging"),i.dataTransfer.effectAllowed="move",i.dataTransfer.setData("text/plain",dragId)}),r.addEventListener("dragover",i=>{if(!dragId||r.dataset.groupOpen==="default")return;i.preventDefault();const o=n.querySelector(`[data-group-open="${dragId}"]`);if(!o||o===r)return;const u=r.getBoundingClientRect(),b=i.clientY>u.top+u.height/2||i.clientX>u.left+u.width/2;n.insertBefore(o,b?r.nextSibling:r),r.classList.add("drag-over")}),r.addEventListener("dragleave",()=>r.classList.remove("drag-over")),r.addEventListener("dragend",()=>{n.querySelectorAll(".dragging,.drag-over").forEach(i=>i.classList.remove("dragging","drag-over"));if(dragId){dragId="";saveOrder()}}),r.addEventListener("drop",i=>{i.preventDefault(),r.classList.remove("drag-over")}),r.addEventListener("click",async i=>{if(i.target.closest("button"))return;if(s.deletingGroupMode){await gt(r.dataset.groupOpen,e,t);return}const o=()=>{if(s.activeGroupId===r.dataset.groupOpen)return;s.activeGroupId=r.dataset.groupOpen,refreshActive(),Ce(t,((s.lastState||{}).selectedId)||"")};if(i.target.closest("[data-group-rename]")){clearTimeout(groupClickTimer),groupClickTimer=window.setTimeout(o,220);return}clearTimeout(groupClickTimer),o()}),r.addEventListener("contextmenu",async i=>{if(i.preventDefault(),i.stopPropagation(),s.busyKey)return;const o=e.find(u=>u.id===r.dataset.groupOpen);if(!o||o.source!=="subscription")return;const u=await showSubscriptionUpdateDialog(o);!u||H(`updateSub:${o.id}`,"\u6B63\u5728\u66F4\u65B0\u8BA2\u9605",async()=>{const b=await UpdateSubscriptionGroupApi(o.id,u.url);P(`\u8BA2\u9605\u5DF2\u66F4\u65B0\uFF0C\u8282\u70B9 ${b.updated||0} \u4E2A`)});})}),n.querySelectorAll("[data-group-rename]").forEach(r=>{r.addEventListener("dblclick",i=>{clearTimeout(groupClickTimer),i.preventDefault(),i.stopPropagation();const o=e.find(u=>u.id===r.dataset.groupRename);if(!!o){if(o.id==="default"){P("\u9ED8\u8BA4\u7EC4\u4E0D\u80FD\u6539\u540D");return}Ot({title:"修改分组",message:"输入新的节点组名称",value:o.name,okText:"保存"}).then(u=>{!u||H(`renameGroup:${o.id}`,"完成后显示输出",async()=>{await An(o.id,u),P("节点组已改名")})})}})}),n.querySelectorAll("[data-group-test]").forEach(r=>{r.addEventListener("click",()=>H("groupTest","正在测试分组",async()=>{s.activeGroupId=r.dataset.groupTest,Oe(e,t),s.testingGroupId=r.dataset.groupTest,Oe(e,t);const o=await showDelayMetricDialog({title:"选择分组测速方式",message:"请选择本次分组批量测速的延迟类型。"});if(!o){s.testingGroupId="";return}const i=await Tn(r.dataset.groupTest,o);P(`${o==="real"?"真连接延迟":"TCP延迟"}已测 ${i.tested} 个，失败 ${i.failed} 个`),s.testingGroupId=""},()=>{s.testingGroupId=""}))}),n.querySelectorAll("[data-group-copy]").forEach(r=>{r.addEventListener("click",()=>H(`copyGroup:${r.dataset.groupCopy}`,"正在复制分组链接",async()=>{const i=await ht(t,r.dataset.groupCopy);if(!i.length){P("当前分组没有可复制的链接");return}await Le(i.join(`
`)),P(`已复制 ${i.length} 条链接`)}))}),n.querySelectorAll("[data-group-delete]").forEach(r=>{r.addEventListener("click",async()=>{await gt(r.dataset.groupDelete,e,t)})})}async function gt(e,t,n){if(s.busyKey)return;const a=t.find(o=>o.id===e);if(!a)return;if(a.id==="default"){P("\u9ED8\u8BA4\u7EC4\u4E0D\u80FD\u5220\u9664");return}const r=n.filter(o=>(o.groupId||"default")===e).length;!await Dt({title:"删除分组",message:`确定删除分组“${a.name||"未命名"}”？`,desc:`该分组包含 ${r} 个节点`,okText:"删除"})||H(`deleteGroup:${e}`,"正在删除分组",async()=>{s.deletingGroupMode=!1,await un(e),s.activeGroupId="default",P("节点组已删除")})}function Ce(e,t){const l=String(s.nodeSearchQuery||"").trim().toLowerCase(),g=((s.lastState&&s.lastState.groups)||[]),f=new Map(g.map(v=>[v.id,v.name||"未命名"])),n=l?e.filter(v=>String(v.name||"").toLowerCase().includes(l)):e.filter(v=>(v.groupId||"default")===s.activeGroupId);c("nodeCount").textContent=l?`总 ${e.length} 个节点 / 匹配 ${n.length} 个`:`总 ${e.length} 个节点`;const a=c("nodes"),r=s.activeGroupId||"default",k=l?"__search__":r,i=s.nodeRenderedGroupId||k,u=Math.max(0,Number(a.scrollTop||0));s.nodeScrollTopByGroup[i]=u||Number(s.nodeScrollTopByGroup[i]||0);const o=Math.max(0,Number(i===k?u:s.nodeScrollTopByGroup[k]||0));s.nodeRenderedGroupId=k;if(!n.length){a.className="nodes empty",a.textContent=l?`暂无匹配节点：${s.nodeSearchQuery}`:e.length?"该组暂无节点":"暂无节点",a.scrollTop=0;return}const renderNode=v=>{const w=v.id===t,x=W(`selectNode:${v.id}`),y=W(`deleteNode:${v.id}`),z=s.testingNodeId===v.id,A=Boolean(s.busyKey||s.testingNodeId),B=`${v.address}:${v.port}`,C=ta(v),D=`流量：${Ne(zt(v))}`,E=f.get(v.groupId||"default")||"默认分组",F=l?`分组：${E} · ${B}`:B;return`
        <article class="node ${l?"":"draggable"} ${w?"selected":""}" data-node-open="${v.id}" draggable="${!l&&!A?"true":"false"}">
          <span class="chip node-protocol" title="${m(v.type)}">${m(v.type)}</span>
          <div class="node-title">
            <button class="node-name-copy" data-copy-node="${v.id}" title="${m(v.name)}" type="button">${m(v.name)}</button>
          </div>
          <div class="node-meta" title="${m(F)}">${m(F)}</div>
          <div class="node-stats">
            <span class="node-delay ${v.delayMs<0?"bad":""}" title="${m(C)}">${m(C)}</span>
            <span class="node-traffic" title="${m(D)}">${m(D)}</span>
          </div>
          <div class="node-actions">
            <button class="select ${w?"selected-action":""} ${x?"is-busy":""}" data-select="${v.id}" ${w||A?"disabled":""}>${w?"已选择":x?"选择中":"选择"}</button>
            <button class="${z?"is-busy":""}" data-test="${v.id}" ${A?"disabled":""}>${z?"测试中":"测试"}</button>
            <button data-edit-node="${v.id}" ${A?"disabled":""}>编辑</button>
            <button data-share-node="${v.id}" ${A?"disabled":""}>分享</button>
            <button data-qr-node="${v.id}" ${A?"disabled":""}>二维码</button>
            <button class="${y?"is-busy":""}" data-delete="${v.id}" ${A?"disabled":""}>${y?"删除中":"删除"}</button>
          </div>
        </article>
      `},findNode=v=>n.find(w=>w.id===v),bindActions=()=>{a.onclick=async v=>{const w=v.target.closest("button");if(!w)return;const x=w.dataset;if(x.select){if(s.busyKey)return;s.busyKey=`selectNode:${x.select}`,te();try{await vn(x.select),s.lastState&&(s.lastState.selectedId=x.select),P("节点已选择")}catch(y){P(J(y))}finally{s.busyKey="",te()}return}if(x.copyNode){H(`copyNode:${x.copyNode}`,"正在复制节点链接",async()=>{const z=String(await ShareLinkApi(x.copyNode)||"").trim();if(!z){P("该节点没有可复制的协议链接");return}await Le(z),P("节点链接已复制")});return}if(x.editNode){H(`editNode:${x.editNode}`,"正在读取节点",async()=>{const y=await NodeDetailApi(x.editNode);ir(y)},null,{skipRefresh:!0});return}if(x.shareNode){H(`shareNode:${x.shareNode}`,"正在复制分享链接",async()=>{const z=String(await ShareLinkApi(x.shareNode)||"").trim();if(!z){P("该节点没有可分享的协议链接");return}await Le(z),P("分享链接已复制")});return}if(x.qrNode){H(`qrNode:${x.qrNode}`,"正在生成二维码",async()=>{const y=await NodeDetailApi(x.qrNode),z=String(await ShareLinkApi(x.qrNode)||"").trim();or("二维码",{...y,raw:z},{qr:!0})},null,{skipRefresh:!0});return}if(x.delete){if(s.busyKey)return;const y=findNode(x.delete),z=(y==null?void 0:y.name)||"该节点";!await Dt({title:"删除节点",message:`确定删除“${z}”？`,desc:"删除后不能恢复，请仔细确认",okText:"删除"})||H(`deleteNode:${x.delete}`,"正在删除节点",async()=>{Ce(e,t),await Ln(x.delete),P("节点已删除")});return}if(x.test){const y=findNode(x.test),z=await prepareStoredNodeTest(y);if(!z)return;const A=await showDelayMetricDialog({title:"选择节点测速方式",message:"请选择本次节点测速的延迟类型。"});if(!A)return;z.delayMetric=A,H("nodeTest","正在测试节点",async()=>{s.testingNodeId=x.test,Ce(e,t);const A=await Sn(x.test,z.delayMetric||"tcp");const ml=(z.delayMetric||"tcp")==="real"?"真连接延迟":"TCP延迟";if(A&&A.delayMs>0){P(`${ml} ${A.delayMs} ms`)}else{P(`${ml}测试失败（超时或节点不可达）`)}s.testingNodeId=""},()=>{s.testingNodeId=""},{preserveNodeScroll:!0})}}};if(n.length>120){a.className=l?"nodes search-results virtual":"nodes virtual",a.innerHTML='<div class="node-virtual-space"><div class="node-virtual-window"></div></div>';const v=a.querySelector(".node-virtual-space"),w=a.querySelector(".node-virtual-window"),x=51,y=8,z=()=>{const A=Math.max(0,Number(a.scrollTop||0));s.nodeScrollTopByGroup[k]=A;const B=Math.max(0,Math.floor(A/x)-y),C=Math.min(n.length,B+Math.ceil(Math.max(1,a.clientHeight)/x)+y*2);v.style.height=`${n.length*x}px`,w.style.transform=`translateY(${B*x}px)`,w.innerHTML=n.slice(B,C).map(renderNode).join("")};let A=0;a.onscroll=()=>{A||(A=requestAnimationFrame(()=>{A=0,z()}))},bindActions();if(!l){let B="",C=[];const D=G=>Math.max(0,Math.min(n.length-1,Math.floor((Math.max(0,Number(a.scrollTop||0))+Math.max(0,G-a.getBoundingClientRect().top))/x))),E=(G,$)=>{const le=[...G],de=le.indexOf(B);if(de<0)return G;const ve=le.splice(de,1)[0],oe=Math.max(0,Math.min(le.length,$));le.splice(oe,0,ve);return le},F=G=>{if(!B)return;G.preventDefault();const $=D(G.clientY),le=E(C,$);le.join("|")!==C.join("|")&&H("reorderNodes","正在保存节点排序",async()=>{await ReorderNodesApi(s.activeGroupId,le),P("节点排序已保存")})};a.ondragstart=G=>{const $=G.target.closest("[data-node-open]");if(!$||s.busyKey||s.testingNodeId){G.preventDefault();return}B=$.dataset.nodeOpen,C=n.map(le=>le.id),$.classList.add("dragging"),G.dataTransfer.effectAllowed="move",G.dataTransfer.setData("text/plain",B)},a.ondragover=G=>{B&&(G.preventDefault(),G.dataTransfer.dropEffect="move")},a.ondrop=F,a.ondragend=()=>{B="",C=[],a.querySelectorAll(".dragging").forEach(G=>G.classList.remove("dragging"))}}const G=()=>{v.style.height=`${n.length*x}px`,a.scrollTop=Math.min(o,Math.max(0,a.scrollHeight-a.clientHeight)),z()};G(),requestAnimationFrame(G);return}a.className=l?"nodes search-results":"nodes",a.innerHTML=n.map(renderNode).join(""),bindActions();if(!l){let v="",w=[];const x=()=>Array.from(a.querySelectorAll("[data-node-open]")).map(A=>A.dataset.nodeOpen).filter(Boolean),y=()=>{const A=x();A.join("|")!==w.join("|")&&H("reorderNodes","正在保存节点排序",async()=>{await ReorderNodesApi(s.activeGroupId,A),P("节点排序已保存")},null,{skipRefresh:!0})},z=A=>{if(!v)return;A.preventDefault();const B=a.querySelector(`[data-node-open="${v}"]`);if(!B)return;const C=Array.from(a.querySelectorAll("[data-node-open]:not(.dragging)"));let D=null;for(const E of C){const F=E.getBoundingClientRect();if(A.clientY<F.top+F.height/2){D=E;break}}a.querySelectorAll(".drag-over").forEach(E=>E.classList.remove("drag-over")),D?(a.insertBefore(B,D),D.classList.add("drag-over")):(a.appendChild(B),C.length&&C[C.length-1].classList.add("drag-over"))};a.ondragover=z,a.ondrop=A=>{v&&A.preventDefault(),a.querySelectorAll(".drag-over").forEach(B=>B.classList.remove("drag-over"))},a.querySelectorAll("[data-node-open]").forEach(A=>{A.addEventListener("dragstart",B=>{if(s.busyKey||s.testingNodeId){B.preventDefault();return}v=A.dataset.nodeOpen,w=x(),A.classList.add("dragging"),B.dataTransfer.effectAllowed="move",B.dataTransfer.setData("text/plain",v)}),A.addEventListener("dragend",()=>{a.querySelectorAll(".dragging,.drag-over").forEach(B=>B.classList.remove("dragging","drag-over"));if(v){v="",y(),w=[]}})})}a.scrollTop=Math.min(o,Math.max(0,a.scrollHeight-a.clientHeight)),a.onscroll=()=>{s.nodeScrollTopByGroup[k]=a.scrollTop}}function It(e){const t=s.settings||{},n=Boolean(t.proxyMap),a=Number(t.proxyMapStartPort||52001),r=it(e),i=r.length?r[r.length-1].port:a;c("proxyMapCount").textContent=`${r.length} 个映射`,c("copyProxyMapAll").disabled=Boolean(s.busyKey)||!r.length,c("copyProxyMapSocks5")&&(c("copyProxyMapSocks5").disabled=Boolean(s.busyKey)||!r.length),c("exportProxyMapCsv")&&(c("exportProxyMapCsv").disabled=Boolean(s.busyKey)||!r.length),c("proxyMapSummary").textContent=n?`已启用 ${a}${r.length>1?`-${i}`:""}`:"未启用",c("proxyMapSummary").classList.toggle("enabled",n);const o=c("proxyMapList"),u=o.querySelector(".proxy-map-table-wrap");u&&(s.proxyMapScrollTop=u.scrollTop);if(!Ze().length){s.proxyMapScrollTop=0,o.className="proxy-map-list empty",o.textContent="请选择映射分组";return}if(!r.length){s.proxyMapScrollTop=0,o.className="proxy-map-list empty",o.textContent="所选分组暂无节点";return}o.className="proxy-map-list",o.innerHTML=`
    <div class="proxy-map-table-wrap">
      <table class="proxy-map-table">
        <thead>
          <tr>
            <th>名称</th>
            <th>类型</th>
            <th>节点地址</th>
            <th>节点延迟</th>
            <th>本地端口 (SOCKS5/HTTP)</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody></tbody>
      </table>
    </div>
  `;const b=o.querySelector(".proxy-map-table-wrap"),S=o.querySelector("tbody"),C=49,N=8,O=()=>{const G=Math.max(0,Number(b.scrollTop||0));s.proxyMapScrollTop=G;const $=Math.max(0,Math.floor(G/C)-N),ae=Math.min(r.length,$+Math.ceil(Math.max(1,b.clientHeight)/C)+N*2),ie=$*C,ge=Math.max(0,(r.length-ae)*C),be=[];ie>0&&be.push(`<tr class="proxy-map-spacer" aria-hidden="true"><td colspan="6" style="height:${ie}px"></td></tr>`);for(let we=$;we<ae;we++){const q=r[we],K=`${q.node.address}:${q.node.port}`,dl=ta(q.node);be.push(`
        <tr>
          <td class="proxy-map-name"><strong title="${m(q.node.name)}">${m(q.node.name)}</strong></td>
          <td><span class="chip" title="${m(q.node.type)}">${m(q.node.type)}</span></td>
          <td class="proxy-map-node-address" title="${m(K)}">${m(K)}</td>
          <td><span class="${Ae(q.node.delayMs)}">${dl}</span></td>
          <td><span class="metric ok copy-value" data-copy-proxy-map="${m(q.address)}" title="点击复制 ${m(q.address)}">${m(q.address)}</span></td>
          <td>
            <div class="proxy-map-row-actions">
              <button class="proxy-map-copy" data-copy-proxy-map="${m(q.address)}" title="复制 127.0.0.1:端口" type="button">复制</button>
              <button class="proxy-map-copy ghost" data-copy-proxy-socks5="socks5://${m(q.address)}" title="复制 socks5://127.0.0.1:端口" type="button">SOCKS5</button>
            </div>
          </td>
        </tr>
      `)}ge>0&&be.push(`<tr class="proxy-map-spacer" aria-hidden="true"><td colspan="6" style="height:${ge}px"></td></tr>`),S.innerHTML=be.join("")};let L=0;b.addEventListener("scroll",()=>{L||(L=requestAnimationFrame(()=>{L=0,O()}))},{passive:!0}),O(),requestAnimationFrame(()=>{b.scrollTop=Math.min(Number(s.proxyMapScrollTop||0),Math.max(0,b.scrollHeight-b.clientHeight)),O()}),o.onclick=G=>{const $=G.target.closest("[data-copy-proxy-map]");if($){H(`copyProxyMap:${$.dataset.copyProxyMap}`,"正在复制代理地址",async()=>{await Le($.dataset.copyProxyMap),P("代理地址已复制")});return}const sk=G.target.closest("[data-copy-proxy-socks5]");if(sk){H(`copyProxySocks5:${sk.dataset.copyProxySocks5}`,"正在复制SOCKS5链接",async()=>{await Le(sk.dataset.copyProxySocks5),P("SOCKS5 链接已复制")});return}}}function sortBatchNodes(key){if(s.batchSortKey===key){s.batchSortAsc=!s.batchSortAsc}else{s.batchSortKey=key,s.batchSortAsc=!0}const asc=s.batchSortAsc;s.testNodes.sort((a,b)=>{if(key==="urlDelayMs"||key==="pingMs"){const va=a[key]||0,vb=b[key]||0,aOk=va>0,bOk=vb>0;if(aOk&&!bOk)return -1;if(!aOk&&bOk)return 1;if(!aOk&&!bOk)return 0;return asc?va-vb:vb-va}if(key==="downloadMbps"||key==="uploadMbps"){const va=Number(a[key]||0),vb=Number(b[key]||0),aOk=va>0,bOk=vb>0;if(aOk&&!bOk)return -1;if(!aOk&&bOk)return 1;if(!aOk&&!bOk)return 0;return asc?va-vb:vb-va}if(key==="country"||key==="name"||key==="type"){const va=String(a[key]||""),vb=String(b[key]||"");return asc?va.localeCompare(vb,"zh-CN"):vb.localeCompare(va,"zh-CN")}return 0}),se()}function se(){c("testNodeCount").textContent=`${s.testNodes.length} 个节点`;const e=c("testNodes");if(!s.testNodes.length){e.className="test-nodes empty",e.textContent="暂无测试节点";return}e.className="test-nodes";const t=s.testNodes.map((n,a)=>{const r=s.testBusyKeys.has(Wt(a))||s.testBusyKey.startsWith(`${s.testRunId}:${a}:`),i=`${n.address}:${n.port}`,o=aa(n,r),u=Boolean(n.error)||n.urlDelayMs<0,b=s.testBusyKey===Xt(a,"speed"),S=Ie(n.pingMs,"ms"),C=Ie(n.urlDelayMs,"ms"),N=na(n.outboundIp),O=Ie(n.downloadMbps,"Mbps"),G=Ie(n.uploadMbps,"Mbps");return`
        <tr class="${r?"testing-row":""}">
          <td class="test-address"><span class="copy-value" data-copy-value="${m(i)}" title="点击复制 ${m(i)}">${m(i)}</span></td>
          <td><span class="chip copy-value" data-copy-value="${m(n.type)}" title="点击复制 ${m(n.type)}">${m(n.type)}</span></td>
          <td><span class="chip copy-value" data-copy-value="${m(n.country)}" title="点击复制 ${m(n.country)}">${m(n.country)}</span></td>
          <td class="test-name"><strong class="copy-value" data-copy-value="${m(n.name)}" title="点击复制 ${m(n.name)}">${m(n.name)}</strong></td>
          <td><span class="${Ae(n.pingMs)} copy-value" data-copy-value="${m(S)}" title="点击复制 ${m(S)}">${m(S)}</span></td>
          <td><span class="${Ae(n.urlDelayMs)} copy-value" data-copy-value="${m(C)}" title="点击复制 ${m(C)}">${m(C)}</span></td>
          <td><span class="${ra(n.outboundIp)} copy-value" data-copy-value="${m(N)}" title="点击复制 ${m(N)}">${m(N)}</span></td>
          <td><span class="${Ae(n.downloadMbps)} copy-value" data-copy-value="${m(O)}" title="点击复制 ${m(O)}">${m(O)}</span></td>
          <td><span class="${Ae(n.uploadMbps)} copy-value" data-copy-value="${m(G)}" title="点击复制 ${m(G)}">${m(G)}</span></td>
          <td><span class="test-status ${u?"bad":""} copy-value" data-copy-value="${m(o)}" title="点击复制 ${m(o)}">${m(o)}</span></td>
          <td>
            <div class="test-actions">
              <button class="${b?"is-busy":""}" data-raw-speed="${a}" ${r||s.batchTesting?"disabled":""}>${b?"测试中":"测试"}</button>
            </div>
          </td>
        </tr>
      `}).join("");const sortIcon=k=>s.batchSortKey===k?(s.batchSortAsc?' <span class="sort-indicator active">▲</span>':' <span class="sort-indicator active">▼</span>'):' <span class="sort-indicator">⇅</span>';e.innerHTML=`
    <div class="test-table-wrap">
      <table class="test-table">
        <thead>
          <tr>
            <th>地址</th>
            <th class="sortable-th" data-sort-key="type" title="点击排序">类型${sortIcon("type")}</th>
            <th class="sortable-th" data-sort-key="country" title="点击排序">地区${sortIcon("country")}</th>
            <th class="sortable-th" data-sort-key="name" title="点击排序">名称${sortIcon("name")}</th>
            <th class="sortable-th" data-sort-key="pingMs" title="点击按 Ping 延迟由低到高排序">Ping 延迟${sortIcon("pingMs")}</th>
            <th class="sortable-th" data-sort-key="urlDelayMs" title="点击按真连接延迟由低到高排序">URL 延迟${sortIcon("urlDelayMs")}</th>
            <th>出口 IP</th>
            <th class="sortable-th" data-sort-key="downloadMbps" title="点击按下载速度排序">下载${sortIcon("downloadMbps")}</th>
            <th class="sortable-th" data-sort-key="uploadMbps" title="点击按上传速度排序">上传${sortIcon("uploadMbps")}</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>${t}</tbody>
      </table>
    </div>
  `,e.querySelectorAll("[data-sort-key]").forEach(th=>{th.addEventListener("click",()=>sortBatchNodes(th.dataset.sortKey))}),e.querySelectorAll("[data-raw-speed]").forEach(n=>{n.addEventListener("click",()=>{s.testStopRequested=!1,Ue(Number(n.dataset.rawSpeed),"speed")})}),e.querySelectorAll("[data-copy-value]").forEach(n=>{n.addEventListener("click",async()=>{const a=String(n.dataset.copyValue||"").trim();if(!!a)try{await Le(a),P("已复制")}catch(r){P(J(r))}})}),te()}function We(){const e=s.toolStatus.nexttrace||{},t=s.toolStatus.iperf3||{};c("mtrStatus")&&(c("mtrStatus").textContent=He(e,"NextTrace")),c("iperfStatus")&&(c("iperfStatus").textContent=He(t,"iperf3"))}function He(e,t){return!e||!e.ready&&!e.errorText&&!e.path?`${t} 检测中`:e.ready?e.version||`${t} 检测中`:`${t} 不可用：${e.errorText||"未知错误"}`}function st(){const e=c("iperfOutput");!e||(e.textContent=s.iperfOutput.length?s.iperfOutput.join(`
`):"暂无输出",e.scrollTop=e.scrollHeight)}function ot(){const e=c("mtrOutput");!e||(e.textContent=s.mtrOutput.length?s.mtrOutput.join(`
`):"暂无输出",e.scrollTop=e.scrollHeight)}function Be(e){const t=c("connections");if(!t)return;const n=Jn(e),a=n.filter(r=>tr(r)).slice(0,60);if(!a.length){t.className="connection-grid empty",t.textContent=n.length?"没有匹配的连接":"暂无连接";return}t.className="connection-grid",t.innerHTML=a.map((r,i)=>{const o=i===0?" featured":"",isBlock=r.rule==="block"||String(r.rule||"").toLowerCase().includes("block"),isProxy=r.rule==="proxy"||String(r.rule||"").toLowerCase().includes("proxy"),isDirect=r.rule==="direct"||String(r.rule||"").toLowerCase().includes("direct"),u=r.error?" rule-error":isBlock?" rule-block":isProxy?" rule-proxy":isDirect?" rule-direct":"",b=r.rule||(r.error?"error":"final"),S=isBlock?'<span class="traffic-blocked">已拦截</span>':r.error?'<span class="traffic-up">异常</span>':'<span class="traffic-muted">正常</span>',p=r.protocol==="UDP"?"chip-udp":"chip-tcp";return'<div class="connection-row'+o+'"><div class="conn-meta"><span class="chip '+p+'">'+m(r.protocol)+'</span><span class="connection-id">#'+m(r.id)+'</span><time class="conn-time">'+m(r.time)+'</time></div><div class="conn-flow"><span class="conn-source" title="来源: '+m(r.source)+'">'+m(r.source)+'</span><span class="conn-arrow" aria-hidden="true">➜</span><strong class="conn-target" title="目标: '+m(r.target)+'">'+m(r.target)+'</strong></div><div class="conn-rule-col"><mark class="conn-rule-chip'+u+'" title="命中规则: '+m(b)+'">'+m(b)+'</mark></div><div class="conn-status-col">'+S+'</div></div>'}).join("")}function Se(e){const t=c("logItems");if(!t)return;const n=e.slice(s.clearedLogIndex),a=n.filter(i=>Bt(i)),r=n.filter(i=>i.level==="ERROR").length;if(c("logTotalBadge").textContent=`${n.length} 条记录`,c("logErrorBadge").textContent=`错误: ${r}`,!a.length){t.onscroll=null,t.className="log-items empty",t.textContent=n.length?"没有匹配的连接":"暂无连接",t.scrollTop=0;return}const i=o=>{const u=o.level==="ERROR"?"level error":"level info";return`
        <div class="${o.level==="ERROR"?"log-row error":"log-row"}">
          <time>${m(o.time)}</time>
          <span class="${u}">[${m(o.level||"INFO")}]</span>
          <code>${m(o.text)}</code>
        </div>
      `};const o=t.scrollHeight-t.clientHeight-t.scrollTop<24;t.onscroll=()=>{s.logScrollTop=Math.max(0,Number(t.scrollTop||0))},t.className="log-items",t.innerHTML=a.map(i).join(""),requestAnimationFrame(()=>{const u=Math.max(0,t.scrollHeight-t.clientHeight);t.scrollTop=s.logAutoScroll||o?u:Math.min(Number(s.logScrollTop||0),u)})}function Xn(e){const t=c("logPreviewItems");if(!t)return;const a=e.slice(s.clearedLogIndex).slice(-8);if(!a.length){t.className="log-preview-items empty",t.textContent="暂无日志";return}t.className="log-preview-items",t.innerHTML=a.map(r=>`
      <div class="log-preview-line ${r.level==="ERROR"?"error":""}">
        <span>[${m(r.level||"INFO")}]</span>
        <code>${m(r.text)}</code>
      </div>
    `).join(""),t.scrollTop=t.scrollHeight}function Jn(e){const t=[];return e.forEach((n,a)=>{const r=Yn(n,a);r&&t.push(r)}),t.slice(-60).reverse()}function Yn(e,t){const n=String(e.text||"");if(!n.includes("connection")&&!n.includes("process"))return null;const a=n.toLowerCase(),isBlock=a.includes("outbound/block")||a.includes("已拦截")||a.includes("block[block]"),r=(e.level==="ERROR"||a.includes("error")||a.includes("was aborted"))&&!isBlock,i=n.match(/from\s+([^\s,?]+)/i),o=n.match(/(?:inbound|outbound)\s+connection\s+to\s+([^\s,?]+)/i)||n.match(/open\s+connection\s+to\s+([^\s,?]+)/i),u=n.match(/outbound\/([^\s:\[]+)|outbound=([^\s,?]+)/i),b=n.match(/rule[_-]set[=:\[]([^\]\s,?]+)|ip[_-]cidr[=:\[]([^\]\s,?]+)/i),S=(i==null?void 0:i[1])||(isBlock?"本机应用":r?zn(n):"未知"),C=Je(o==null?void 0:o[1])||er(n)||"未知";if(S==="未知"&&C==="未知"&&!r&&!isBlock)return null;const N=a.includes(" udp ")?"UDP":"TCP";let O=(u==null?void 0:u[1])||(u==null?void 0:u[2])||"";if(isBlock||O.startsWith("block"))O="block";const G=(b==null?void 0:b[1])||(b==null?void 0:b[2])||O||(r?"error":"final");return{id:String(t+1).padStart(4,"0"),time:e.time||"",protocol:N,source:S,target:C,rule:G,error:r,searchText:`${S} ${C} ${G} ${N} ${n}`.toLowerCase()}}function zn(e){const t=e.match(/from\s+([^\s,?]+)/i);return(t==null?void 0:t[1])||"未知"}function er(e){const t=e.match(/([0-9a-f.:]+:\d+)->([0-9a-f.:]+:\d+)/i);if(t)return Je(t[2]);const n=e.match(/write\s+\w+\s+([^\s,?]+)/i);return Je(n==null?void 0:n[1])}function Je(e){const t=String(e||"").trim().replace(/[.;??]+$/g,"");return!t||t==="-"||t.toLowerCase()==="respond"?"":t}function tr(e){const t=s.connectionQuery.trim().toLowerCase(),n=s.connectionType,a=s.connectionRule;if(t&&!e.searchText.includes(t)||n!=="all"&&(n==="error"&&!e.error||n!=="error"&&e.protocol.toLowerCase()!==n))return!1;if(a!=="all"){const r=String(e.rule||"").toLowerCase();if(a==="error")return e.error;if(!r.includes(a))return!1}return!0}function Bt(e){const t=s.logQuery.trim().toLowerCase();return s.logLevel!=="all"&&e.level!==s.logLevel?!1:t?`${e.time} ${e.level} ${e.text}`.toLowerCase().includes(t):!0}function yt(){var e;const t=s.fullLogs||(((e=s.lastState)==null?void 0:e.logs)||[]);return t.slice(s.clearedLogIndex).filter(n=>Bt(n)).map(n=>`${n.time} [${n.level}] ${n.text}`).join(`
`)}function nr(e,t){const n=new Blob([t],{type:"text/plain;charset=utf-8"}),a=URL.createObjectURL(n),r=document.createElement("a");r.href=a,r.download=e,document.body.appendChild(r),r.click(),r.remove(),URL.revokeObjectURL(a)}function rr(){const e=new Date,t=n=>String(n).padStart(2,"0");return`${e.getFullYear()}${t(e.getMonth()+1)}${t(e.getDate())}-${t(e.getHours())}${t(e.getMinutes())}${t(e.getSeconds())}`}function ke(e){return e==="global"?"global":"rule"}function Ze(e=s.settings||{}){const t=Array.isArray(e.proxyMapGroupIds)?e.proxyMapGroupIds:[],n=e.proxyMapGroupId||"",a=[];return[...t,n].forEach(r=>{r&&!a.includes(r)&&a.push(r)}),a}function Nt(){return Array.from(document.querySelectorAll("[data-proxy-map-group]:checked")).map(e=>e.dataset.proxyMapGroup).filter(Boolean)}function ar(e){const t=Ze();return t.length?(e||[]).filter(n=>t.includes(n.groupId||"default")):[]}function it(e){var n;const t=Number(((n=s.settings)==null?void 0:n.proxyMapStartPort)||52001),a=Ze();if(!a.length)return[];return(e||[]).map((r,i)=>{const o=t+i;return{node:r,port:o,address:`127.0.0.1:${o}`}}).filter(r=>a.includes(r.node.groupId||"default"))}function sr(e){return it(e).map(t=>t.address).join(`
`)}async function ht(e,t){return(await ShareGroupLinksApi(t)).map(n=>String(n||"").trim()).filter(Boolean)}function W(e){return s.busyKey===e}function Ye(){var e,t;return s.speedTrafficMB=mt((e=c("speedTrafficMB"))==null?void 0:e.value,Cn,20),s.speedDurationSec=mt((t=c("speedDurationSec"))==null?void 0:t.value,Bn,3),{trafficMB:s.speedTrafficMB,durationSec:s.speedDurationSec}}function mt(e,t,n){const a=Number(e);return t.includes(a)?a:n}
function copyProxyMapSocks5(){var t;const nodes=((t=s.lastState)==null?void 0:t.nodes)||[];const r=it(nodes);if(!r.length){P("暂无可复制的代理映射");return}const txt=r.map(m=>`socks5://${m.address}`).join("\n");H("copyProxyMapSocks5","正在复制SOCKS5映射",async()=>{await Le(txt);P(`已复制 ${r.length} 条 SOCKS5 链接`)})}
function exportProxyMapCsv(){var t;const nodes=((t=s.lastState)==null?void 0:t.nodes)||[];const r=it(nodes);if(!r.length){P("暂无可导出的代理映射");return}let csv="\uFEFF序号,代理类型,代理主机,代理端口,代理账号,代理密码,节点备注\r\n";r.forEach((m,idx)=>{const n=`"${String(m.node.name||'').replace(/"/g,'""')}"`;csv+=`${idx+1},socks5,127.0.0.1,${m.port},,,${n}\r\n`});nr(`SanTian-ProxyMap-${rr()}.csv`,csv);P(`已导出 ${r.length} 个端口映射配置到 CSV`)}
function filterDeadNodes(){if(!s.testNodes.length){P("暂无测试节点");return}const before=s.testNodes.length;const alive=s.testNodes.filter(n=>n.urlDelayMs>0||n.pingMs>0||(!n.error&&n.urlDelayMs===0&&n.pingMs===0&&n.status==="待测试"));const deadCount=before-alive.length;if(deadCount<=0){P("当前列表中未发现失效节点");return}s.testNodes=alive,se(),P(`已成功剔除 ${deadCount} 个失效节点，剩余 ${s.testNodes.length} 个可用节点`)}
async function copyWorkingNodes(){if(!s.testNodes.length){P("暂无测试节点");return}const working=s.testNodes.filter(n=>n.urlDelayMs>0||n.pingMs>0);if(!working.length){P("未找到测试可用的节点，请先进行批量测试");return}const raws=working.map(n=>String(n.raw||"").trim()).filter(Boolean);await Le(raws.join("\n")),P(`已复制 ${raws.length} 个可用节点链接`)}
async function saveBatchToGroup(){if(!s.testNodes.length){P("暂无测试节点");return}const working=s.testNodes.filter(n=>n.urlDelayMs>0||n.pingMs>0);const targets=working.length>0?working:s.testNodes;const raws=targets.map(n=>String(n.raw||"").trim()).filter(Boolean);if(!raws.length){P("暂无可保存的有效节点");return}const d=new Date(),pad=num=>String(num).padStart(2,"0");const defaultName=`批量优选-${d.getFullYear()}${pad(d.getMonth()+1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;const groupName=await Ot({title:"存为新分组",message:`将 ${raws.length} 个${working.length>0?"已测试可用":"测试"}节点保存为新分组：`,value:defaultName,okText:"创建并保存"});if(!groupName)return;await H("saveBatchToGroup","正在创建分组并导入节点",async()=>{const newGroup=await pn(groupName);const gid=newGroup.id||"default";await fn(raws.join("\n"),gid,true);s.activeGroupId=gid;await Me();Tt("home");P(`已成功创建分组【${groupName}】并导入 ${raws.length} 个节点！`)})}
function te(){var n;const e=Boolean(s.busyKey);Y(importBtn,W("import"),pt(nodeInput.value.trim())==="subscription"?"\u5BFC\u5165\u8BA2\u9605\u4E2D":"\u5BFC\u5165\u4E2D",rn()),c("importBtn").disabled=e,Y(copyProxyMapAll,W("copyProxyMapAll"),"\u590D\u5236\u4E2D","\u590D\u5236\u5168\u90E8"),c("copyProxyMapAll").disabled=e||!it(((n=s.lastState)==null?void 0:n.nodes)||[]).length,c("copyProxyMapSocks5")&&(c("copyProxyMapSocks5").disabled=e||!it(((n=s.lastState)==null?void 0:n.nodes)||[]).length),c("exportProxyMapCsv")&&(c("exportProxyMapCsv").disabled=e||!it(((n=s.lastState)==null?void 0:n.nodes)||[]).length),Y(pasteTestNodes,W("pasteTestNodes"),"\u8BFB\u53D6\u4E2D","\u526A\u8D34\u677F\u7C98\u8D34"),c("pasteTestNodes").disabled=e,Y(c("batchDelayTest"),W("batchDelayTest")||s.batchTesting,"\u6D4B\u8BD5\u4E2D","\u6279\u91CF\u6D4B\u8BD5"),c("batchDelayTest").disabled=e||s.batchTesting,c("stopBatchTest").disabled=!Jt(),c("filterDeadNodes")&&(c("filterDeadNodes").disabled=e||s.batchTesting||!s.testNodes.length),c("copyWorkingNodes")&&(c("copyWorkingNodes").disabled=e||s.batchTesting||!s.testNodes.length),c("saveBatchToGroup")&&(c("saveBatchToGroup").disabled=e||s.batchTesting||!s.testNodes.length),Y(c("startMTR"),s.mtrRunning,"\u8FD0\u884C\u4E2D","\u5F00\u59CB"),c("startMTR")&&(c("startMTR").disabled=e||s.mtrRunning),c("stopMTR")&&(c("stopMTR").disabled=!s.mtrRunning),c("mtrTarget")&&(c("mtrTarget").disabled=e||s.mtrRunning),c("mtrNetworkMode")&&(c("mtrNetworkMode").disabled=e||s.mtrRunning),c("batchNetworkMode")&&(c("batchNetworkMode").disabled=e||s.batchTesting),Y(startIperfUpload,s.iperfRunning,"\u8FD0\u884C\u4E2D","\u5F00\u59CB\u4E0A\u4F20"),Y(startIperfDownload,s.iperfRunning,"\u8FD0\u884C\u4E2D","\u5F00\u59CB\u4E0B\u8F7D"),c("startIperfUpload")&&(c("startIperfUpload").disabled=e||s.iperfRunning),c("startIperfDownload")&&(c("startIperfDownload").disabled=e||s.iperfRunning),c("stopIperf")&&(c("stopIperf").disabled=!s.iperfRunning),c("iperfTarget")&&(c("iperfTarget").disabled=e||s.iperfRunning),c("iperfParallel")&&(c("iperfParallel").disabled=e||s.iperfRunning);const t=e||s.testBusyKey.endsWith(":speed");c("speedTrafficMB").disabled=t,c("speedDurationSec").disabled=t,Y(copyLogs,W("copyLogs"),"\u590D\u5236\u4E2D","\u590D\u5236"),c("copyLogs").disabled=e,Y(restartProxy,W("restartProxy"),"\u91CD\u542F\u4E2D","\u91CD\u542F"),c("restartProxy").disabled=e,_t(),ne()}function _t(){const e=Boolean(s.busyKey);Y(pickAppProxyFolder,W("pickAppProxyFolder"),"\u9009\u62E9\u4E2D","\u9009\u62E9\u6587\u4EF6\u5939"),Y(pickAppProxyApp,W("pickAppProxyApp"),"\u9009\u62E9\u4E2D","\u9009\u62E9\u5E94\u7528"),c("pickAppProxyFolder")&&(c("pickAppProxyFolder").disabled=e),c("pickAppProxyApp")&&(c("pickAppProxyApp").disabled=e)}function Y(e,t,n,a){if(!e||!e.dataset)return;e.dataset.idleText||(e.dataset.idleText=a||e.textContent),e.classList.toggle("is-busy",t),e.textContent=t?n:a||e.dataset.idleText}function ne(){const e=c("activity");if(!e)return;const t=Boolean(s.busyKey||s.testingNodeId||s.testingGroupId||s.batchTesting||s.ipBackfilling||s.testBusyKey||s.testBusyKeys.size||s.mtrRunning||s.iperfRunning),n=s.busyText||(s.iperfRunning?"iperf3 运行中":s.mtrRunning?"MTR 运行中":s.batchTesting?"完成后显示输出":s.ipBackfilling?"未知 IP":dt(s.testBusyKey.split(":")[2]||""));activityText.textContent=n||"\u5904\u7406\u4E2D",e.classList.toggle("hidden",!t)}function $e(e,t){if(s.busyKey=e||"",s.busyText=t||"",Rt()){te();return}if(s.lastState){Ge(s.lastState);return}te()}function Dt({title:e,message:t,desc:n,okText:a="确认",cancelText:r="取消"}){const i=c("confirmDialog"),o=c("confirmOk"),u=c("confirmCancel");return c("confirmTitle").textContent=e,c("confirmMessage").textContent=t,c("confirmDesc").textContent=n||"",o.textContent=a,u.textContent=r,i.classList.remove("hidden"),o.focus(),new Promise(b=>{const S=$=>{i.classList.add("hidden"),o.removeEventListener("click",C),u.removeEventListener("click",N),window.removeEventListener("keydown",G),b($)},C=()=>S(!0),N=()=>S(!1),G=$=>{$.key==="Escape"&&S(!1)};o.addEventListener("click",C),u.addEventListener("click",N),window.addEventListener("keydown",G)})}function showSubscriptionUpdateDialog(e){const t=c("subscriptionUpdateDialog"),n=c("subscriptionUpdateURL"),a=c("subscriptionUpdateOk"),r=c("subscriptionUpdateCancel"),i=c("subscriptionUpdateClose"),o=c("subscriptionCopyURL"),u=String((e==null?void 0:e.url)||"").trim();c("subscriptionUpdateMessage").textContent="是否更新订阅："+((e==null?void 0:e.name)||"未命名")+"？",n.value=u,t.classList.remove("hidden"),n.focus(),n.select();return new Promise(b=>{const S=$=>{t.classList.add("hidden"),a.removeEventListener("click",C),r.removeEventListener("click",N),i.removeEventListener("click",N),o.removeEventListener("click",O),window.removeEventListener("keydown",he),b($)},C=()=>{const $=String(n.value||"").trim();if(!$){P("请输入订阅链接"),n.focus();return}try{new URL($)}catch{P("订阅链接格式不正确"),n.focus();return}S({url:$})},N=()=>S(null),O=async()=>{const $=String(n.value||"").trim();if(!$){P("没有可复制的订阅链接");return}await Le($),P("订阅链接已复制")},he=$=>{$.key==="Escape"&&S(null),$.key==="Enter"&&(document.activeElement===n&&($.preventDefault(),C()))};a.addEventListener("click",C),r.addEventListener("click",N),i.addEventListener("click",N),o.addEventListener("click",O),window.addEventListener("keydown",he)})}function showDelayMetricDialog(e={}){const t=c("delayMetricDialog"),n=c("delayMetricTcp"),a=c("delayMetricReal"),r=c("delayMetricCancel");c("delayMetricTitle").textContent=e.title||"选择测速方式",c("delayMetricMessage").textContent=e.message||"请选择本次要测试的延迟类型。",t.classList.remove("hidden"),n.focus();return new Promise(i=>{const o=u=>{t.classList.add("hidden"),n.removeEventListener("click",b),a.removeEventListener("click",S),r.removeEventListener("click",C),window.removeEventListener("keydown",N),i(u)},b=()=>o("tcp"),S=()=>o("real"),C=()=>o(""),N=u=>{u.key==="Escape"&&o("")};n.addEventListener("click",b),a.addEventListener("click",S),r.addEventListener("click",C),window.addEventListener("keydown",N)})}window.__sanTianChooseDelayMetric=showDelayMetricDialog;function showTestModeDialog(e={}){const t=c("testModeDialog"),n=c("testModeDisableTun"),a=c("testModeSwitchSystem"),r=c("testModeCancel");c("testModeTitle").textContent=e.title||"测试前需要处理 TUN",c("testModeMessage").textContent=e.message||"TUN 模式会接管本机流量，建议先关闭 TUN 或切换为系统代理后再使用本地直连测试。",c("testModeDesc").textContent=e.desc||"选择后软件会自动应用设置，再开始测试。",t.classList.remove("hidden"),n.focus();return new Promise(i=>{const o=u=>{t.classList.add("hidden"),n.removeEventListener("click",b),a.removeEventListener("click",S),r.removeEventListener("click",C),window.removeEventListener("keydown",O),i(u)},b=()=>o("disableTun"),S=()=>o("systemProxy"),C=()=>o(""),O=u=>{u.key==="Escape"&&o("")};n.addEventListener("click",b),a.addEventListener("click",S),r.addEventListener("click",C),window.addEventListener("keydown",O)})}function isSocks5TestNode(e){const t=String((e==null?void 0:e.type)||"").toLowerCase(),n=String((e==null?void 0:e.raw)||"").trim().toLowerCase();return t==="socks5"||t==="socks"||n.startsWith("socks://")||n.startsWith("socks5://")||/^[^:\s]+:\d+:[^:\s]+:[^\s]+$/.test(n)}function currentProxyState(){const e=s.pendingSettings||s.settings||((s.lastState||{}).settings)||{};return{systemProxy:Boolean(e.systemProxy),tun:Boolean(e.tun),running:Boolean((s.lastState||{}).running)}}async function applyTestProxyChoice(e){if(e==="disableTun"){c("tunProxy").checked=!1,c("systemProxy").checked=!1}else if(e==="systemProxy"){c("tunProxy").checked=!1,c("systemProxy").checked=!0}else return!1;const t=lt();s.pendingSettings=t,s.settings=t,s.applyingSettings=!0,s.settingsSaveRunning=!0,s.settingsSaveQueued=!1,$e("settings","正在应用设置");try{await Pt(t),s.settings=t,await Me().catch(()=>{})}finally{s.applyingSettings=!1,s.settingsSaveRunning=!1,s.settingsSaveQueued=!1,s.pendingSettings=null,$e("",""),te()}return!0}async function prepareBatchRawTest(e=[]){
  let t = currentProxyState();
  const r = (c("batchNetworkMode") ? c("batchNetworkMode").value : "local") === "proxy" ? "proxy" : "local";
  if (r === "proxy") {
    if (!t.running) {
      await Dt({
        title: "代理未连接",
        message: "代理测试模式需要通过已连接的代理进行测试。",
        desc: "请先在主页选择节点并启动代理连接后，再进行代理测试。",
        okText: "知道了",
        cancelText: "取消"
      });
      return null;
    }
    return {
      trafficMB: Ye().trafficMB,
      durationSec: Ye().durationSec,
      useSystemProxy: true,
      directOnly: false,
      proxyMode: true,
      fastDelay: true
    };
  }
  return {
    trafficMB: Ye().trafficMB,
    durationSec: Ye().durationSec,
    useSystemProxy: false,
    directOnly: true,
    proxyMode: false,
    fastDelay: true
  };
}
function rawTestOptionsForNode(e) {
  const t = s.currentRawTestOptions || {};
  const networkMode = (c("batchNetworkMode") ? c("batchNetworkMode").value : "local") === "proxy" ? "proxy" : "local";
  const proxyMode = Boolean(t.proxyMode || networkMode === "proxy");
  if (proxyMode) {
    return {
      trafficMB: Ye().trafficMB,
      durationSec: Ye().durationSec,
      useSystemProxy: true,
      directOnly: false,
      proxyMode: true,
      fastDelay: true
    };
  }
  return {
    trafficMB: Ye().trafficMB,
    durationSec: Ye().durationSec,
    useSystemProxy: false,
    directOnly: true,
    proxyMode: false,
    fastDelay: true
  };
}
function normalizeTestNodeList(e) {
  return Array.isArray(e) ? e.filter(Boolean) : (e ? [e] : []);
}
async function prepareStoredNodeTest(e=[]) {
  return { directOnly: true, fastDelay: true };
}
window.__sanTianPrepareStoredNodeTest=prepareStoredNodeTest;window.__sanTianRefreshState=Me;window.__sanTianRenderState=Ge;function showUpdateChoiceDialog(){const e=c("updateChoiceDialog"),t=c("updateChoiceCore"),n=c("updateChoiceRules"),appBtn=c("updateChoiceApp"),a=c("updateChoiceAll"),r=c("updateChoiceCancel"),i=c("updateChoiceClose");if(appBtn){appBtn.classList.toggle("update-available",Boolean(s.lastState&&s.lastState.appUpdateAvailable))}return e.classList.remove("hidden"),t.focus(),new Promise(o=>{const u=$=>{e.classList.add("hidden"),t.removeEventListener("click",b),n.removeEventListener("click",S),appBtn&&appBtn.removeEventListener("click",UA),a.removeEventListener("click",C),r.removeEventListener("click",N),i&&i.removeEventListener("click",N),window.removeEventListener("keydown",G),o($)},b=()=>u("core"),S=()=>u("rules"),UA=()=>u("app"),C=()=>u("all"),N=()=>u(""),G=$=>{$.key==="Escape"&&u("")};t.addEventListener("click",b),n.addEventListener("click",S),appBtn&&appBtn.addEventListener("click",UA),a.addEventListener("click",C),r.addEventListener("click",N),i&&i.addEventListener("click",N),window.addEventListener("keydown",G)})}function Ot({title:e,message:t,value:n="",okText:a="保存"}){const r=c("groupDialog"),i=c("groupNameInput"),o=c("groupOk"),u=c("groupCancel");return c("groupDialogTitle").textContent=e,c("groupDialogMessage").textContent=t,o.textContent=a,i.value=n,r.classList.remove("hidden"),i.focus(),i.select(),new Promise(b=>{const S=$=>{r.classList.add("hidden"),o.removeEventListener("click",C),u.removeEventListener("click",N),window.removeEventListener("keydown",G),b($)},C=()=>{const $=i.value.trim();if(!$){P("请输入节点组名称"),i.focus();return}S($)},N=()=>S(""),G=$=>{$.key==="Escape"&&S(""),$.key==="Enter"&&C()};o.addEventListener("click",C),u.addEventListener("click",N),window.addEventListener("keydown",G)})}function or(e,t,n={}){const a=c("nodeLinkDialog"),r=c("nodeLinkText"),i=c("nodeQrBox"),o=c("nodeLinkClose"),u=c("nodeLinkCopy"),b=String((t==null?void 0:t.raw)||"").trim();a.querySelector(".node-link-card").classList.toggle("qr-only",Boolean(n.qr)),c("nodeLinkTitle").textContent=e,nodeLinkName.textContent=(t==null?void 0:t.name)||"\u672A\u547D\u540D\u8282\u70B9",r.value=b||`${(t==null?void 0:t.name)||""}
${(t==null?void 0:t.type)||""}
${(t==null?void 0:t.address)||""}:${(t==null?void 0:t.port)||""}`.trim(),r.classList.toggle("hidden",Boolean(n.qr)),u.classList.toggle("hidden",Boolean(n.qr)),i.classList.toggle("hidden",!n.qr),i.innerHTML="",n.qr&&(b?(i.className="node-qr-box",i.innerHTML=Ut(b)):(i.className="node-qr-box empty",i.textContent="该节点没有可生成二维码的原始链接")),a.classList.remove("hidden"),n.qr?o.focus():(r.focus(),r.select());const S=()=>{a.classList.add("hidden"),o.removeEventListener("click",S),u.removeEventListener("click",C),window.removeEventListener("keydown",O)},C=()=>H("copyNodeDialog","正在复制节点信息",async()=>{await Le(r.value),P(b?"\u8282\u70B9\u94FE\u63A5\u5DF2\u590D\u5236":"\u8282\u70B9\u4FE1\u606F\u5DF2\u590D\u5236")},null,{skipRefresh:!0}),O=G=>{G.key==="Escape"&&S()};o.addEventListener("click",S),u.addEventListener("click",C),window.addEventListener("keydown",O)}function ir(e){const t=c("nodeEditDialog"),n=c("nodeEditForm"),a=c("nodeEditSave"),r=c("nodeEditCancel"),l=c("nodeEditClose"),i=cr(e);c("nodeEditProtocol").textContent=`${ct(i.protocol)} / ${(e==null?void 0:e.address)||""}:${(e==null?void 0:e.port)||""}`,Lr(n,i),t.classList.remove("hidden");const o=n.querySelector("input, select, textarea");o&&o.focus();const u=()=>{t.classList.add("hidden"),a.removeEventListener("click",b),r.removeEventListener("click",S),l&&l.removeEventListener("click",S),n.removeEventListener("submit",N),window.removeEventListener("keydown",O)},b=()=>H(`updateNode:${e.id}`,"正在保存节点",async()=>{var me,Ee,R;const G=vr(n,i),$=wr(i.protocol,G),he=(R=(Ee=(me=window.go)==null?void 0:me.main)==null?void 0:Ee.App)==null?void 0:R.UpdateNode;if(!he)throw new Error("\u7F16\u8F91\u8282\u70B9\u63A5\u53E3\u672A\u5C31\u7EEA\uFF0C\u8BF7\u91CD\u542F\u5F00\u53D1\u6A21\u5F0F");await he(e.id,$),u(),P("\u8282\u70B9\u5DF2\u4FDD\u5B58")}),S=()=>u(),N=G=>{G.preventDefault(),b()},O=G=>{G.key==="Escape"&&u()};a.addEventListener("click",b),r.addEventListener("click",S),l&&l.addEventListener("click",S),n.addEventListener("submit",N),window.addEventListener("keydown",O)}function cr(e){const t=(e==null?void 0:e.outbound)||{},n=String((e==null?void 0:e.raw)||"").trim(),a=kr((e==null?void 0:e.type)||t.type||n.split("://")[0]),d=["shadowsocks","vmess","vless","trojan","hysteria2","tuic","anytls","wireguard","naive","socks5","http"].includes(a);if(n.startsWith("{")&&!d)return{protocol:"json",values:{raw:n}};const r=ce(t.tls),i=ce(r.reality),o=ce(r.utls),u=ce(r.ech),b=ce(t.transport),S=ce(b.headers),C=ce(t.headers),N=ce(t.udp_over_tcp),O=ce(t.multiplex),G=ce(t.obfs),$=lr(n,a),he=i.enabled?"reality":r.enabled?"tls":$.security||"none",me={name:(e==null?void 0:e.name)||$.name||"",server:(e==null?void 0:e.address)||$.server||t.server||"",port:(e==null?void 0:e.port)||$.port||t.server_port||"",username:Z($.username,t.username),password:Z($.password,t.password),method:Z($.method,t.method),uuid:Z($.uuid,t.uuid),privateKey:Z($.privateKey,t.private_key),peerPublicKey:Z($.peerPublicKey,$.publicKey,t.peer_public_key,t.public_key),preSharedKey:Z($.preSharedKey,t.pre_shared_key),reserved:Z($.reserved,Array.isArray(t.reserved)?t.reserved.join(", "):""),localAddress:Z($.localAddress,Array.isArray(t.local_address)?t.local_address.join(","):t.local_address),mtu:Z($.mtu,t.mtu,1280),flow:Z($.flow,t.flow),alterId:Z($.alterId,t.alter_id,0),vmessSecurity:Z($.vmessSecurity,t.security,"auto"),encryption:Z($.encryption,t.encryption,"none"),security:he,sni:Z($.sni,r.server_name),alpn:Z($.alpn,Array.isArray(r.alpn)?r.alpn.join(","):""),certificate:Z($.certificate,Array.isArray(r.certificate)?r.certificate.join(`
`):r.certificate),certificateFingerprint:Z($.certificateFingerprint,Array.isArray(r.certificate_public_key_sha256)?r.certificate_public_key_sha256.join(`
`):r.certificate_public_key_sha256),echConfigList:Z($.echConfigList,Array.isArray(u.config)?u.config.join(`
`):u.config),allowInsecure:Boolean($.allowInsecure||r.insecure),fingerprint:Z($.fingerprint,o.fingerprint),publicKey:Z($.publicKey,i.public_key),shortId:Z($.shortId,i.short_id),spiderX:Z($.spiderX),mldsa65Verify:Z($.mldsa65Verify),transport:Z($.transport,b.type,"tcp"),rawType:Z($.rawType,"none"),finalMask:Z($.finalMask,""),host:Z($.host,S.Host,C.Host),path:Z($.path,b.path,t.path),serviceName:Z($.serviceName,b.service_name),network:Z($.network,t.network,"tcp"),packetEncoding:Z($.packetEncoding,t.packet_encoding,""),udpOverTcp:Boolean($.udpOverTcp||N.enabled),mux:Boolean($.mux||O.enabled),tlsEnabled:Boolean($.tlsEnabled||r.enabled),obfsPassword:Z($.obfsPassword,G.password),hopPorts:Z($.hopPorts,Array.isArray(t.server_ports)?t.server_ports.join(","):t.server_ports),hopInterval:Z($.hopInterval,t.hop_interval),upMbps:Z($.upMbps,t.up_mbps),downMbps:Z($.downMbps,t.down_mbps),congestionControl:Z($.congestionControl,t.congestion_control),udpRelayMode:Z($.udpRelayMode,t.udp_relay_mode),zeroRtt:Boolean($.zeroRtt||t.zero_rtt_handshake),insecureConcurrency:Z($.insecureConcurrency,t.insecure_concurrency),quic:Boolean($.quic||t.quic),quicCongestionControl:Z($.quicCongestionControl,t.quic_congestion_control)};return{protocol:a,values:me}}function lr(e,t){try{if(t==="vmess"){const o=JSON.parse(et(e.replace(/^vmess:\/\//i,"")));return{name:o.ps||"",server:o.add||"",port:o.port||"",uuid:o.id||"",alterId:o.aid||0,vmessSecurity:o.scy||"auto",security:o.tls||"none",sni:o.sni||"",alpn:o.alpn||"",fingerprint:o.fp||o.fingerprint||"",transport:o.net||"tcp",host:o.host||"",path:o.path||"",serviceName:o.serviceName||o.service_name||"",mux:j(o.mux)||j(o.multiplex),packetEncoding:o.packetEncoding||o.packet_encoding||"",rawType:o.rawType||o.raw_type||"none",finalMask:o.finalMask||o.finalmask||""}}if(t==="shadowsocks")return dr(e);if(t==="wireguard")return pr(e);if(t==="naive")return ur(e);const n=new URL(e.replace(/^hy2:\/\//i,"hysteria2://")),a=n.searchParams,r=ee(n.username),i={name:ee(n.hash.replace(/^#/,"")),server:n.hostname.replace(/^\[(.*)\]$/,"$1"),port:n.port||"",username:ee(n.username),password:r,uuid:ee(n.username),security:a.get("security")||a.get("tls")||"none",encryption:a.get("encryption")||"none",sni:a.get("sni")||a.get("servername")||a.get("peer")||"",alpn:a.get("alpn")||"",allowInsecure:j(a.get("allowInsecure"))||j(a.get("insecure")),fingerprint:a.get("fp")||a.get("fingerprint")||"",publicKey:a.get("pbk")||"",certificate:a.get("certificate")||a.get("cert")||a.get("fixed_certificate")||"",certificateFingerprint:a.get("certificate_public_key_sha256")||a.get("cert_fp")||a.get("certFingerprint")||"",echConfigList:a.get("ech_config")||a.get("echConfigList")||a.get("ech")||"",shortId:a.get("sid")||"",spiderX:a.get("spx")||a.get("spiderX")||"",mldsa65Verify:a.get("pqv")||a.get("mldsa65Verify")||"",transport:a.get("type")||"tcp",rawType:a.get("rawType")||a.get("headerType")||"none",finalMask:a.get("finalMask")||a.get("finalmask")||"",network:a.get("network")||a.get("net")||"tcp",packetEncoding:a.get("packetEncoding")||a.get("packet_encoding")||"",udpOverTcp:j(a.get("udp_over_tcp"))||j(a.get("uot")),mux:j(a.get("mux"))||j(a.get("multiplex")),host:a.get("host")||"",path:a.get("path")||"",serviceName:a.get("serviceName")||a.get("service_name")||"",flow:a.get("flow")||"",tlsEnabled:n.protocol==="https:",obfsPassword:a.get("obfs-password")||a.get("obfs_password")||a.get("obfs")||"",hopPorts:a.get("mport")||a.get("ports")||a.get("server_ports")||"",hopInterval:a.get("hop")||a.get("hop_interval")||"",upMbps:a.get("upmbps")||a.get("up_mbps")||a.get("up")||"",downMbps:a.get("downmbps")||a.get("down_mbps")||a.get("down")||"",congestionControl:a.get("congestion_control")||a.get("congestion")||"",udpRelayMode:a.get("udp_relay_mode")||a.get("udpRelayMode")||"",zeroRtt:j(a.get("zero_rtt_handshake"))||j(a.get("zeroRtt"))};return t==="tuic"&&(i.password=ee(n.password)),i}catch{return{}}}function dr(e){const t={};try{const n=new URL(e);t.name=ee(n.hash.replace(/^#/,"")),t.server=n.hostname.replace(/^\[(.*)\]$/,"$1"),t.port=n.port||"",t.network=n.searchParams.get("network")||n.searchParams.get("net")||"tcp",t.udpOverTcp=j(n.searchParams.get("udp_over_tcp"))||j(n.searchParams.get("uot")),t.mux=j(n.searchParams.get("mux"))||j(n.searchParams.get("multiplex")),t.transport=n.searchParams.get("type")||"tcp",t.rawType=n.searchParams.get("rawType")||n.searchParams.get("headerType")||"none",t.finalMask=n.searchParams.get("finalMask")||n.searchParams.get("finalmask")||"",t.security=n.searchParams.get("security")||n.searchParams.get("tls")||"none",t.sni=n.searchParams.get("sni")||"",t.allowInsecure=j(n.searchParams.get("allowInsecure"))||j(n.searchParams.get("insecure"));let a=n.username;a&&!n.password?a=et(a):(a||n.password)&&(a=`${ee(n.username)}:${ee(n.password)}`);const r=String(a||"").split(":");return t.method=r.shift()||"",t.password=r.join(":"),t}catch{try{const n=et(e.replace(/^ss:\/\//i,"").split("#")[0]),[a,r]=n.split("@"),i=String(a||"").split(":");t.method=i.shift()||"",t.password=i.join(":");const o=String(r||"").match(/^(.*):(\d+)$/);o&&(t.server=o[1],t.port=o[2])}catch{return t}}return t}function pr(e){try{const t=new URL(e),n=t.searchParams;return{name:ee(t.hash.replace(/^#/,"")),server:t.hostname.replace(/^\[(.*)\]$/,"$1"),port:t.port||"",privateKey:ee(t.username)||n.get("private_key")||n.get("privateKey")||"",peerPublicKey:n.get("public_key")||n.get("publicKey")||n.get("peer_public_key")||n.get("peerPublicKey")||"",preSharedKey:n.get("pre_shared_key")||n.get("preSharedKey")||n.get("psk")||"",reserved:n.get("reserved")||"0, 0, 0",localAddress:n.get("address")||n.get("local_address")||n.get("localAddress")||"",mtu:n.get("mtu")||"1280",network:n.get("network")||n.get("net")||"tcp"}}catch{return{}}}function ur(e){try{const t=e.replace(/^naive\+https:\/\//i,"naive://").replace(/^naive\+quic:\/\//i,"naive://"),n=new URL(t),a=n.searchParams;return{name:ee(n.hash.replace(/^#/,"")),server:n.hostname.replace(/^\[(.*)\]$/,"$1"),port:n.port||"",username:ee(n.username),password:ee(n.password),sni:a.get("sni")||a.get("servername")||"",alpn:a.get("alpn")||"",allowInsecure:j(a.get("allowInsecure"))||j(a.get("insecure")),udpOverTcp:j(a.get("udp_over_tcp"))||j(a.get("uot")),quic:j(a.get("quic")),congestionControl:a.get("quic_congestion_control")||a.get("congestion")||"",insecureConcurrency:a.get("insecure_concurrency")||""}}catch{return{}}}function Lr(e,t){if(t.protocol==="json"){e.innerHTML=`
      <label class="field node-edit-wide">
        <span>原始 JSON</span>
        <textarea name="raw" class="node-edit-raw">${m(t.values.raw||"")}</textarea>
      </label>
    `;return}e.innerHTML=Zt(t.protocol).map(n=>hr(n,t.values[n.key],t.values)).join(""),Ve(e),e.querySelectorAll('[name="security"], [name="transport"]').forEach(n=>{n.addEventListener("change",()=>Ve(e))}),e.querySelectorAll('[name="tlsEnabled"]').forEach(n=>{n.addEventListener("change",()=>Ve(e))}),e.querySelectorAll("[data-generate-uuid]").forEach(n=>{n.addEventListener("click",()=>{const a=e.querySelector(`[name="${n.dataset.generateUuid}"]`);a&&(a.value=crypto.randomUUID?crypto.randomUUID():mr())})}),e.querySelectorAll("[data-certificate-toggle]").forEach(n=>{n.addEventListener("click",()=>{const a=e.querySelector("[data-certificate-panel]");!a||a.classList.toggle("hidden")})}),e.querySelectorAll("[data-certificate-fetch]").forEach(n=>{n.addEventListener("click",()=>{P("获取证书功能暂未接入，请手动粘贴 PEM 证书")})})}function Zt(e){const t=[{type:"section",label:"基础配置"},{key:"name",label:"名称 (remarks)",required:!0},{key:"server",label:"地址 (address)",required:!0},{key:"port",label:"端口 (port)",type:"number",required:!0}],n=[{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"TLS 类型",type:"select",options:[["none","无"],["tls","TLS"],["reality","Reality"]]},{key:"sni",label:"SNI / serverName",show:"tls"},{key:"alpn",label:"ALPN",show:"tls"},{key:"allowInsecure",label:"跳过证书验证",type:"checkbox",show:"tls"},{key:"fingerprint",label:"uTLS 指纹",show:"tls"},{key:"publicKey",label:"Reality PublicKey",show:"reality"},{key:"shortId",label:"Reality ShortId",show:"reality"}],a=[{type:"section",label:"底层传输方式 (transport)"},{key:"transport",label:"传输方式 (network)",type:"select",options:[["tcp","raw / tcp"],["ws","WebSocket"],["grpc","gRPC"],["httpupgrade","HTTPUpgrade"],["http","HTTP/2"]]},{key:"rawType",label:"raw 伪装类型",type:"select",options:[["none","none"],["http","http"]]},{key:"finalMask",label:"Finalmask"},{key:"host",label:"WS Host",show:"ws"},{key:"path",label:"WS Path",show:"ws"},{key:"serviceName",label:"gRPC ServiceName",show:"grpc"}],r=[{key:"udpOverTcp",label:"UDP over TCP",type:"checkbox"},{key:"mux",label:"启用 Mux 多路复用",type:"checkbox"}];return e==="shadowsocks"?t.concat([{key:"password",label:"密码 (password)",required:!0},{key:"method",label:"加密方式 (encryption)",type:"select",required:!0,options:fr()}],r):e==="vmess"?t.concat([{key:"uuid",label:"用户 ID (id)",required:!0,action:"uuid"},{key:"alterId",label:"额外 ID (alterId)",type:"number"},{key:"vmessSecurity",label:"加密方式 (security)",type:"select",options:[["auto","auto"],["zero","zero"],["none","none"],["aes-128-gcm","aes-128-gcm"],["chacha20-poly1305","chacha20-poly1305"]]},{key:"mux",label:"开启 Mux 多路复用",type:"checkbox"},{type:"section",label:"底层传输方式 (transport)"},{key:"transport",label:"传输协议 (network)",type:"select",options:[["tcp","raw"],["ws","ws"],["grpc","grpc"],["httpupgrade","httpupgrade"],["http","h2"]]},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["none",""] ,["tls","tls"]]},{key:"sni",label:"SNI",show:"tls-only"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr(),show:"tls-only"},{key:"alpn",label:"Alpn",type:"select",options:yr(),show:"tls-only"},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]],show:"tls-only"},{key:"echConfigList",label:"EchConfigList",type:"textarea",show:"tls-only"},{key:"certificate",label:"固定证书",type:"certificate",show:"tls-only"}]):e==="vless"?t.concat([{key:"uuid",label:"用户 ID (id)",required:!0,action:"uuid"},{key:"flow",label:"流控 (flow)",type:"select",options:[["",""],["xtls-rprx-vision","xtls-rprx-vision"]]},{key:"encryption",label:"加密方式 (encryption)",type:"select",options:[["none","none"]]},{key:"mux",label:"启用 Mux 多路复用",type:"checkbox"},{type:"section",label:"底层传输方式 (transport)"},{key:"transport",label:"传输方式 (network)",type:"select",options:[["tcp","raw"],["raw","raw"]]},{key:"rawType",label:"raw 伪装类型",type:"select",options:[["none","none"],["http","http"]]},{key:"finalMask",label:"Finalmask"},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["none",""],["tls","tls"],["reality","reality"]]},{key:"sni",label:"SNI",show:"tls"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr(),show:"tls"},{key:"publicKey",label:"PublicKey",show:"reality"},{key:"shortId",label:"ShortId",show:"reality"},{key:"spiderX",label:"SpiderX",show:"reality"},{key:"mldsa65Verify",label:"Mldsa65Verify",show:"reality"},{key:"alpn",label:"Alpn",type:"select",options:yr(),show:"tls"},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]],show:"tls"},{key:"echConfigList",label:"EchConfigList",type:"textarea",show:"tls"},{key:"certificate",label:"固定证书",type:"certificate",show:"tls"}]):e==="trojan"?t.concat([{key:"password",label:"密码 (password)",required:!0},{key:"mux",label:"开启 Mux 多路复用",type:"checkbox"},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["none",""] ,["tls","tls"],["reality","reality"]]},{key:"sni",label:"SNI",show:"tls"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr(),show:"tls"},{key:"alpn",label:"Alpn",type:"select",options:yr(),show:"tls-only"},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]],show:"tls-only"},{key:"echConfigList",label:"EchConfigList",type:"textarea",show:"tls-only"},{key:"certificate",label:"固定证书",type:"certificate",show:"tls-only"},{key:"publicKey",label:"PublicKey",show:"reality"},{key:"shortId",label:"ShortId",show:"reality"}]):e==="hysteria2"?t.concat([{key:"password",label:"密码 (password)",required:!0},{key:"obfsPassword",label:"混淆密码 (obfs password)"},{key:"hopPorts",label:"跳跃端口范围"},{key:"hopInterval",label:"端口跳跃时间间隔"},{key:"upMbps",label:"Hysteria 最大带宽 Up",type:"number"},{key:"downMbps",label:"Hysteria 最大带宽 Down",type:"number"},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["tls","tls"]]},{key:"sni",label:"SNI"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr()},{key:"alpn",label:"Alpn",type:"select",options:yr()},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]]},{key:"echConfigList",label:"EchConfigList",type:"textarea"},{key:"certificate",label:"固定证书",type:"certificate"}]):e==="tuic"?t.concat([{key:"uuid",label:"用户 ID (id)",required:!0,action:"uuid"},{key:"password",label:"密码 (password)",required:!0},{key:"congestionControl",label:"拥塞控制算法",type:"select",options:[["",""],["cubic","cubic"],["new_reno","new_reno"],["bbr","bbr"]]},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["tls","tls"]]},{key:"sni",label:"SNI"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr()},{key:"alpn",label:"Alpn",type:"select",options:yr()},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]]},{key:"echConfigList",label:"EchConfigList",type:"textarea"},{key:"certificate",label:"固定证书",type:"certificate"}]):e==="anytls"?t.concat([{key:"password",label:"密码 (password)",required:!0},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["tls","tls"],["reality","reality"]]},{key:"sni",label:"SNI"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr()},{key:"publicKey",label:"Reality PublicKey",show:"reality"},{key:"shortId",label:"Reality ShortId",show:"reality"},{key:"alpn",label:"ALPN",type:"select",options:yr()},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]]},{key:"echConfigList",label:"ECHConfigList",type:"textarea"},{key:"certificate",label:"固定证书",type:"certificate"}]):e==="wireguard"?t.concat([{key:"privateKey",label:"PrivateKey",required:!0},{key:"peerPublicKey",label:"PublicKey",required:!0},{key:"preSharedKey",label:"PreSharedKey"},{key:"reserved",label:"Reserved"},{key:"localAddress",label:"Address (IPv4,IPv6)",required:!0},{key:"mtu",label:"MTU",type:"number"}]):e==="http"?t.concat([{key:"username",label:"用户名 (可选)"},{key:"password",label:"密码 (可选)"},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["none",""] ,["tls","tls"]]},{key:"sni",label:"SNI",show:"tls-only"},{key:"fingerprint",label:"Fingerprint",type:"select",options:gr(),show:"tls-only"},{key:"alpn",label:"Alpn",type:"select",options:yr(),show:"tls-only"},{key:"allowInsecure",label:"跳过证书验证 (allowInsecure)",type:"select",options:[["false","false"],["true","true"]],show:"tls-only"},{key:"echConfigList",label:"EchConfigList",type:"textarea",show:"tls-only"},{key:"certificate",label:"固定证书",type:"certificate",show:"tls-only"}]):e==="naive"?t.concat([{key:"username",label:"用户名",required:!0},{key:"password",label:"密码 (password)",required:!0},{key:"quic",label:"QUIC",type:"checkbox"},{key:"congestionControl",label:"拥塞控制算法",type:"select",options:[["",""] ,["bbr","bbr"],["bbr2","bbr2"],["cubic","cubic"],["reno","reno"]]},{key:"insecureConcurrency",label:"不安全并发",type:"number"},{key:"udpOverTcp",label:"UDP over TCP",type:"checkbox"},{type:"section",label:"传输层安全 (TLS)"},{key:"security",label:"传输层安全 (TLS)",type:"select",options:[["tls","tls"]]},{key:"sni",label:"SNI"},{key:"echConfigList",label:"EchConfigList",type:"textarea"},{key:"certificate",label:"固定证书",type:"certificate"}]):e==="socks5"?t.concat([{key:"username",label:"用户名 (可选)"},{key:"password",label:"密码 (可选)"}]):t.concat([{key:"username",label:"用户名"},{key:"password",label:"密码"}],r.slice(0,1),a,n.filter(i=>i.key!=="publicKey"&&i.key!=="shortId"))}function fr(){return[["aes-128-gcm","aes-128-gcm"],["aes-256-gcm","aes-256-gcm"],["chacha20-ietf-poly1305","chacha20-ietf-poly1305"],["2022-blake3-aes-128-gcm","2022-blake3-aes-128-gcm"],["2022-blake3-aes-256-gcm","2022-blake3-aes-256-gcm"],["2022-blake3-chacha20-poly1305","2022-blake3-chacha20-poly1305"]]}function gr(){return[["","默认"],["chrome","chrome"],["firefox","firefox"],["safari","safari"],["ios","ios"],["android","android"],["edge","edge"],["360","360"],["qq","qq"],["random","random"],["randomized","randomized"]]}function yr(){return[["","默认"],["h3","h3"],["h2","h2"],["http/1.1","http/1.1"],["h3,h2","h3,h2"],["h2,http/1.1","h2,http/1.1"],["h3,h2,http/1.1","h3,h2,http/1.1"]]}function hr(e,t,n={}){var u;if(e.type==="section")return`<div class="node-edit-section">${m(e.label)}</div>`;const a=[`name="${m(e.key)}"`,e.required?"required":"",e.key==="port"?'min="1" max="65535"':""].filter(Boolean).join(" "),r=e.show?` data-show="${m(e.show)}"`:"",i=e.action==="uuid"?`<button class="ghost node-edit-inline-btn" type="button" data-generate-uuid="${m(e.key)}">生成</button>`:"",o=i?" node-edit-with-action":"";if(e.type==="select"){const b=t===!1?"false":t===!0?"true":String(t!=null?t:"");return`
      <label class="field"${r}>
        <span>${m(e.label)}</span>
        <select ${a}>${e.options.map(([S,C])=>`<option value="${m(S)}" ${b===S?"selected":""}>${m(C)}</option>`).join("")}</select>
      </label>
    `}if(e.type==="textarea")return`
      <label class="field node-edit-wide"${r}>
        <span>${m(e.label)}</span>
        <textarea ${a} class="node-edit-textarea">${m(t!=null?t:"")}</textarea>
      </label>
    `;if(e.type==="certificate"){const b=String(t||"").trim()!=="";return`
      <div class="node-edit-cert-row node-edit-wide">
        <span class="node-edit-cert-label">${m(e.label)}</span>
        <button class="node-edit-cert-toggle" type="button" data-certificate-toggle>
          <span data-certificate-status>设置证书</span>
          <strong aria-hidden="true">+</strong>
        </button>
      </div>
      <div class="node-edit-cert-panel node-edit-wide hidden" data-certificate-panel>
        <div class="node-edit-cert-title">固定证书（二选一，填写即可）</div>
        <p>指定固定证书后，将禁用“跳过证书验证”。</p>
        <p>获取证书可能因自签名证书或未受信任的 CA 而失败。</p>
        <div class="node-edit-cert-actions">
          <button class="primary" type="button" data-certificate-fetch="cert">获取证书</button>
          <button class="primary" type="button" data-certificate-fetch="chain">获取证书链</button>
        </div>
        <label class="switch node-edit-switch">
          <input name="allowUnsafeCertificateFetch" type="checkbox" />
          <span>允许不安全获取证书（自签名）</span>
        </label>
        <label class="field node-edit-wide">
          <span>证书指纹（SHA-256）</span>
          <input name="certificateFingerprint" type="text" value="${m((u=n.certificateFingerprint)!=null?u:"")}" />
        </label>
        <label class="field node-edit-wide">
          <span>完整证书（链），PEM 格式</span>
          <textarea name="${m(e.key)}" class="node-edit-cert-textarea">${m(t!=null?t:"")}</textarea>
        </label>
      </div>
    `}return e.type==="checkbox"?`
      <label class="switch node-edit-switch"${r}>
        <input ${a} type="checkbox" ${t?"checked":""} />
        <span>${m(e.label)}</span>
      </label>
    `:`
    <label class="field"${r}>
      <span>${m(e.label)}</span>
      <div class="node-edit-control${o}">
        <input ${a} type="${e.type||"text"}" value="${m(t!=null?t:"")}" />
        ${i}
      </div>
    </label>
  `}function Ve(e){var r,i,o;const t=((r=e.querySelector('[name="security"]'))==null?void 0:r.value)||"none",n=((i=e.querySelector('[name="transport"]'))==null?void 0:i.value)||"tcp",a=Boolean((o=e.querySelector('[name="tlsEnabled"]'))==null?void 0:o.checked);e.querySelectorAll("[data-show]").forEach(u=>{const b=u.dataset.show,S=b===t||b===n||b==="https"&&a||b==="tls"&&(t==="tls"||t==="reality")||b==="tls-only"&&t==="tls";u.classList.toggle("hidden",!S)})}function mr(){return"xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g,e=>{const t=Math.floor(Math.random()*16);return(e==="x"?t:t&3|8).toString(16)})}function vr(e,t){var r;if(t.protocol==="json"){const i=(r=e.querySelector('[name="raw"]'))==null?void 0:r.value.trim();if(!i)throw new Error("原始 JSON 不能为空");return{raw:i}}const n={};e.querySelectorAll("input, select, textarea").forEach(i=>{n[i.name]=i.type==="checkbox"?i.checked:i.value.trim()});const a=Number(n.port);if(!Number.isInteger(a)||a<1||a>65535)throw new Error("端口必须在 1-65535 之间");n.port=String(a);for(const i of Zt(t.protocol))if(i.required&&!n[i.key])throw new Error(`${i.label}不能为空`);return br(n.certificateFingerprint),xr(t.protocol,n),n}function br(e){const t=String(e||"").trim();if(!t)return;const n=t.split(/[\s,;|]+/).filter(Boolean);for(const a of n)try{const r=a.replace(/-/g,"+").replace(/_/g,"/");if(atob(r).length!==32)throw new Error("invalid length")}catch{throw new Error("证书指纹必须是 SHA-256 公钥指纹的 base64 值，长度为 32 字节")}}function xr(e,t){const n=[],a=b=>{var S;return String((S=t[b])!=null?S:"").trim()!==""},r=(b,S)=>a(b)&&String(t[b]).trim()!==S,i=b=>fe(t[b]),o=(b,S)=>{b&&n.push(S)};if(e==="socks5"&&(()=>{o(r("transport","tcp"),"传输协议"),o(r("rawType","none"),"raw 伪装类型"),o(a("finalMask"),"Finalmask"),o(a("host"),"WS Host"),o(a("path"),"WS Path"),o(a("serviceName"),"gRPC ServiceName"),o(r("security","none"),"TLS 类型"),o(a("sni"),"SNI"),o(a("alpn"),"ALPN"),o(i("allowInsecure"),"跳过证书验证"),o(a("fingerprint"),"uTLS 指纹")})(),e==="http"&&(o(r("transport","tcp"),"传输协议"),o(r("rawType","none"),"raw 伪装类型"),o(a("finalMask"),"Finalmask"),o(a("serviceName"),"gRPC ServiceName")),e==="vmess"&&(o(r("rawType","none"),"raw 伪装类型"),o(a("finalMask"),"Finalmask")),n.length>0)throw new Error(`${ct(e)} 不支持以下字段：${n.join("，")}，请移除后再保存`)}function wr(e,t){if(e==="json")return t.raw;const n=encodeURIComponent(t.name||ct(e)),a=Mr(t.server,t.port);if(e==="shadowsocks"){const o=new URLSearchParams;return ze(o,t,{uot:!0,mux:!0}),`ss://${bt(`${t.method}:${t.password}`)}@${a}${ie(o)}#${n}`}if(e==="vmess"){const o={v:"2",ps:t.name,add:t.server,port:t.port,id:t.uuid,aid:Number(t.alterId||0),scy:t.vmessSecurity||"auto",net:t.transport||"tcp",type:"none",host:t.host||"",path:t.path||"",tls:t.security==="tls"?"tls":"",sni:t.sni||"",alpn:t.alpn||"",fp:t.fingerprint||"",allowInsecure:!t.certificate&&!t.certificateFingerprint&&fe(t.allowInsecure),certificate:t.certificate||"",certificate_public_key_sha256:t.certificateFingerprint||"",ech_config:t.echConfigList||"",mux:Boolean(t.mux)};return t.transport==="grpc"&&(o.serviceName=t.serviceName||""),`vmess://${bt(JSON.stringify(o))}`}if(e==="vless")return`${e}://${z(t.uuid)}@${a}${ie(vt(t))}#${n}`;if(e==="trojan")return`trojan://${z(t.password)}@${a}${ie(vt(t))}#${n}`;if(e==="hysteria2"){const o=new URLSearchParams;return _(o,"sni",t.sni),_(o,"obfs-password",t.obfsPassword),_(o,"mport",t.hopPorts),_(o,"hop",t.hopInterval),_(o,"upmbps",t.upMbps),_(o,"downmbps",t.downMbps),_(o,"fp",t.fingerprint),_(o,"alpn",t.alpn),_(o,"certificate",t.certificate),_(o,"certificate_public_key_sha256",t.certificateFingerprint),_(o,"ech_config",t.echConfigList),!t.certificate&&!t.certificateFingerprint&&fe(t.allowInsecure)&&o.set("insecure","1"),`hysteria2://${z(t.password)}@${a}${ie(o)}#${n}`}if(e==="tuic"){const o=new URLSearchParams;return _(o,"sni",t.sni),_(o,"congestion_control",t.congestionControl),_(o,"fp",t.fingerprint),_(o,"alpn",t.alpn),_(o,"certificate",t.certificate),_(o,"certificate_public_key_sha256",t.certificateFingerprint),_(o,"ech_config",t.echConfigList),!t.certificate&&!t.certificateFingerprint&&fe(t.allowInsecure)&&o.set("allowInsecure","1"),`tuic://${z(t.uuid)}:${z(t.password)}@${a}${ie(o)}#${n}`}if(e==="anytls"){const o=new URLSearchParams;return t.security&&o.set("security",t.security),_(o,"sni",t.sni),_(o,"alpn",t.alpn),_(o,"fp",t.fingerprint),_(o,"pbk",t.security==="reality"?t.publicKey:""),_(o,"sid",t.security==="reality"?t.shortId:""),_(o,"certificate",t.certificate),_(o,"certificate_public_key_sha256",t.certificateFingerprint),_(o,"ech_config",t.echConfigList),!t.certificate&&!t.certificateFingerprint&&fe(t.allowInsecure)&&o.set("allowInsecure","1"),`anytls://${z(t.password)}@${a}${ie(o)}#${n}`}if(e==="wireguard"){const o=new URLSearchParams;return _(o,"public_key",t.peerPublicKey),_(o,"pre_shared_key",t.preSharedKey),_(o,"reserved",t.reserved),_(o,"address",t.localAddress),_(o,"mtu",t.mtu),`wireguard://${z(t.privateKey)}@${a}${ie(o)}#${n}`}if(e==="http"){const o=new URLSearchParams,u=t.security==="tls"||fe(t.tlsEnabled);u&&(_(o,"sni",t.sni),_(o,"fp",t.fingerprint),_(o,"alpn",t.alpn),_(o,"certificate",t.certificate),_(o,"certificate_public_key_sha256",t.certificateFingerprint),_(o,"ech_config",t.echConfigList),fe(t.allowInsecure)&&o.set("allowInsecure","1"));const b=t.username?`${z(t.username)}${t.password?`:${z(t.password)}`:""}@`:"";return`${u?"https":"http"}://${b}${a}${ie(o)}#${n}`}if(e==="naive"){const o=new URLSearchParams;return _(o,"sni",t.sni),_(o,"quic_congestion_control",t.congestionControl),_(o,"insecure_concurrency",t.insecureConcurrency),_(o,"certificate",t.certificate),_(o,"certificate_public_key_sha256",t.certificateFingerprint),_(o,"ech_config",t.echConfigList),t.quic&&o.set("quic","1"),t.udpOverTcp&&o.set("udp_over_tcp","1"),`naive://${z(t.username)}:${z(t.password)}@${a}${ie(o)}#${n}`}const r=new URLSearchParams;return ze(r,t,{uot:!0,transport:!0,tls:!0}),`socks5://${t.username?`${z(t.username)}${t.password?`:${z(t.password)}`:""}@`:""}${a}${ie(r)}#${n}`}function vt(e){const t=new URLSearchParams;return ze(t,e,{mux:!0,transport:!0,tls:!0}),e.security&&e.security!=="none"&&t.set("security",e.security),_(t,"encryption",e.encryption),_(t,"sni",e.sni),fe(e.allowInsecure)&&t.set("allowInsecure","1"),_(t,"fp",e.fingerprint),_(t,"pbk",e.security==="reality"?e.publicKey:""),_(t,"sid",e.security==="reality"?e.shortId:""),_(t,"spx",e.security==="reality"?e.spiderX:""),_(t,"pqv",e.security==="reality"?e.mldsa65Verify:""),_(t,"flow",e.flow),_(t,"certificate",e.certificate),_(t,"certificate_public_key_sha256",e.certificateFingerprint),_(t,"ech_config",e.echConfigList),_(t,"host",e.transport==="ws"?e.host:""),_(t,"path",e.transport==="ws"?e.path:""),_(t,"serviceName",e.transport==="grpc"?e.serviceName:""),t}function ze(e,t,n={}){n.transport&&t.transport&&t.transport!=="tcp"&&e.set("type",t.transport),_(e,"rawType",t.rawType&&t.rawType!=="none"?t.rawType:""),_(e,"finalMask",t.finalMask),n.tls&&t.security&&t.security!=="none"&&e.set("security",t.security),_(e,"sni",t.sni),_(e,"alpn",t.alpn),fe(t.allowInsecure)&&e.set("allowInsecure","1"),_(e,"fp",t.fingerprint),n.uot&&t.udpOverTcp&&e.set("udp_over_tcp","1"),n.mux&&t.mux&&e.set("mux","1"),_(e,"network",t.network&&t.network!=="tcp"?t.network:"")}function kr(e){const t=String(e||"").toLowerCase();return t==="ss"||t==="shadowsocks"?"shadowsocks":t==="hy2"||t==="hysteria2"?"hysteria2":t==="socks"||t==="socks5"?"socks5":t==="wireguard"?"wireguard":t==="naive"||t==="naive+https"||t==="naive+quic"?"naive":["vmess","vless","trojan","tuic","anytls","http"].includes(t)?t:t||"json"}function ct(e){return{shadowsocks:"Shadowsocks",vmess:"VMess",vless:"VLESS",trojan:"Trojan",hysteria2:"Hysteria2",tuic:"TUIC",anytls:"AnyTLS",wireguard:"WireGuard",naive:"Naive",socks5:"SOCKS5",http:"HTTP",json:"JSON Outbound"}[e]||e}function Mr(e,t){const n=String(e||"").trim();return`${n.includes(":")&&!n.startsWith("[")?`[${n}]`:n}:${t}`}function _(e,t,n){n!=null&&String(n).trim()!==""&&e.set(t,String(n).trim())}function ie(e){const t=e.toString();return t?`?${t}`:""}function ce(e){return e&&typeof e=="object"?e:{}}function Z(...e){for(const t of e)if(t!=null&&String(t)!=="")return t;return""}function z(e){return encodeURIComponent(String(e||""))}function ee(e){try{return decodeURIComponent(String(e||""))}catch{return String(e||"")}}function j(e){return["1","true","yes"].includes(String(e||"").toLowerCase())}function fe(e){return typeof e=="boolean"?e:j(e)}function bt(e){const t=new TextEncoder().encode(e);let n="";return t.forEach(a=>{n+=String.fromCharCode(a)}),btoa(n).replace(/\+/g,"-").replace(/\//g,"_").replace(/=+$/g,"")}function et(e){let t=String(e||"").replace(/-/g,"+").replace(/_/g,"/");for(;t.length%4;)t+="=";const n=atob(t),a=Uint8Array.from(n,r=>r.charCodeAt(0));return new TextDecoder().decode(a)}function Ut(e){try{const t=at(0,"M");return t.addData(e),t.make(),t.createSvgTag(6,8).replace("<svg ",'<svg class="node-qr-svg" ')}catch(t){return`<div class="node-qr-error">${m(J(t))}</div>`}}async function Tr(){var n,a;const e=s.activeGroupId||"default",t=(a=(n=window.go)==null?void 0:n.main)==null?void 0:a.App;if(!(t!=null&&t.ExportGroupLinks))throw new Error("\u5BFC\u51FA\u63A5\u53E3\u672A\u5C31\u7EEA\uFF0C\u8BF7\u91CD\u542F\u5F00\u53D1\u6A21\u5F0F");return t.ExportGroupLinks(e)}function Sr(e){return new Promise((t,n)=>{const a=new Image,r=220;a.onload=()=>{const i=document.createElement("canvas");i.width=r,i.height=r;const o=i.getContext("2d");o.fillStyle="#fff",o.fillRect(0,0,r,r),o.drawImage(a,0,0,r,r);const u=i.toDataURL("image/png").split(",")[1]||"";t(_r(u))},a.onerror=()=>n(new Error("二维码生成失败")),a.src=`data:image/svg+xml;charset=utf-8,${encodeURIComponent(Ut(e))}`})}function $r(e,t){const n={};return n["[Content_Types].xml"]=pe(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Default Extension="png" ContentType="image/png"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/drawings/drawing1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawing+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`),n["_rels/.rels"]=pe(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`),n["xl/_rels/workbook.xml.rels"]=pe(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`),n["xl/workbook.xml"]=pe(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="${qt(Or(e))}" sheetId="1" r:id="rId1"/></sheets>
</workbook>`),n["xl/styles.xml"]=pe(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="1"><font><sz val="11"/><name val="Microsoft YaHei UI"/></font></fonts>
<fills count="1"><fill><patternFill patternType="none"/></fill></fills>
<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
</styleSheet>`),n["xl/worksheets/sheet1.xml"]=pe(Pr(t)),n["xl/worksheets/_rels/sheet1.xml.rels"]=pe(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing" Target="../drawings/drawing1.xml"/>
</Relationships>`),n["xl/drawings/drawing1.xml"]=pe(Er(t)),n["xl/drawings/_rels/drawing1.xml.rels"]=pe(Rr(t)),t.forEach((a,r)=>{n[`xl/media/qr${r+1}.png`]=a.qr}),Ir(n)}function Pr(e){let n=`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<dimension ref="A1:C${e.length+1}"/><sheetViews><sheetView workbookViewId="0"/></sheetViews>
<sheetFormatPr defaultRowHeight="18"/><cols><col min="1" max="1" width="28" customWidth="1"/><col min="2" max="2" width="96" customWidth="1"/><col min="3" max="3" width="20" customWidth="1"/></cols><sheetData>`;return n+=xt(1,["名称","链接","二维码"],18),e.forEach((a,r)=>{n+=xt(r+2,[a.name,a.link,""],92)}),n+='</sheetData><drawing r:id="rId1"/><pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/></worksheet>',n}function xt(e,t,n){const a=t.map((r,i)=>`<c r="${String.fromCharCode(65+i)}${e}" t="inlineStr"><is><t>${qt(r)}</t></is></c>`).join("");return`<row r="${e}" ht="${n}" customHeight="1">${a}</row>`}function Er(e){let n=`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`;return e.forEach((a,r)=>{const i=r+1,o=r+1;n+=`<xdr:oneCellAnchor><xdr:from><xdr:col>2</xdr:col><xdr:colOff>160000</xdr:colOff><xdr:row>${i}</xdr:row><xdr:rowOff>70000</xdr:rowOff></xdr:from><xdr:ext cx="${1047750}" cy="${1047750}"/><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="${o}" name="QRCode ${o}"/><xdr:cNvPicPr><a:picLocks noChangeAspect="1"/></xdr:cNvPicPr></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId${o}"/><a:stretch><a:fillRect/></a:stretch></xdr:blipFill><xdr:spPr><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></xdr:spPr></xdr:pic><xdr:clientData/></xdr:oneCellAnchor>`}),n+="</xdr:wsDr>",n}function Rr(e){let t='<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">';return e.forEach((n,a)=>{const r=a+1;t+=`<Relationship Id="rId${r}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/qr${r}.png"/>`}),`${t}</Relationships>`}function Ir(e){const t=new TextEncoder,n=[],a=[];let r=0;Object.entries(e).forEach(([u,b])=>{const S=t.encode(u),C=b instanceof Uint8Array?b:new Uint8Array(b),N=Ar(C);n.push(Qe(67324752,[[20,2],[0,2],[0,2],[0,2],[0,2],[N,4],[C.length,4],[C.length,4],[S.length,2],[0,2]]),S,C),a.push({nameBytes:S,fileBytes:C,crc:N,offset:r}),r+=30+S.length+C.length});const i=r;a.forEach(u=>{n.push(Qe(33639248,[[20,2],[20,2],[0,2],[0,2],[0,2],[0,2],[u.crc,4],[u.fileBytes.length,4],[u.fileBytes.length,4],[u.nameBytes.length,2],[0,2],[0,2],[0,2],[0,2],[0,4],[u.offset,4]]),u.nameBytes),r+=46+u.nameBytes.length});const o=r-i;return n.push(Qe(101010256,[[0,2],[0,2],[a.length,2],[a.length,2],[o,4],[i,4],[0,2]])),Br(n)}function Qe(e,t){const n=4+t.reduce((o,[,u])=>o+u,0),a=new Uint8Array(n),r=new DataView(a.buffer);r.setUint32(0,e,!0);let i=4;return t.forEach(([o,u])=>{u===2?r.setUint16(i,o,!0):r.setUint32(i,o>>>0,!0),i+=u}),a}function Ar(e){let t=-1;for(const n of e)t=t>>>8^Cr[(t^n)&255];return(t^-1)>>>0}const Cr=Array.from({length:256},(e,t)=>{let n=t;for(let a=0;a<8;a+=1)n=n&1?3988292384^n>>>1:n>>>1;return n>>>0});function Br(e){const t=e.reduce((r,i)=>r+i.length,0),n=new Uint8Array(t);let a=0;return e.forEach(r=>{n.set(r,a),a+=r.length}),n}function pe(e){return new TextEncoder().encode(e)}function Nr(e){let t="";for(let n=0;n<e.length;n+=32768)t+=String.fromCharCode(...e.subarray(n,n+32768));return btoa(t)}function _r(e){const t=atob(e),n=new Uint8Array(t.length);for(let a=0;a<t.length;a+=1)n[a]=t.charCodeAt(a);return n}function qt(e){return String(e!=null?e:"").replaceAll("&","&amp;").replaceAll("<","&lt;").replaceAll(">","&gt;").replaceAll('"',"&quot;")}function Dr(e){return String(e||"分组名称").replace(/[\\/:*?"<>|]/g,"_").trim()||"分组名称"}function Or(e){return String(e||"分组名称").replace(/[\\/?*[\]:]/g,"_").slice(0,31)||"分组名称"}function updateDirectDomainsVisibility(){const e=ke(s.selectedMode)==="rule",t=c("directDomains"),n=c("directDomainsError"),a=c("ruleSummary");c("homePage")?.classList.toggle("direct-domains-hidden",!e),a&&a.classList.toggle("hidden",!e),c("directDomainsLabel")&&(c("directDomainsLabel").textContent="自定义直连域名"),t&&(t.disabled=!e),!e&&(t&&t.classList.remove("invalid"),n&&(n.textContent=""))}function validateDirectDomains(){const e=c("directDomains"),t=c("directDomainsError");if(!e)return{ok:!0,value:""};if(ke(s.selectedMode)!=="rule")return{ok:!0,value:String(e.value||s.settings.directDomains||((s.lastState||{}).settings||{}).directDomains||"")};const n=String(e.value||""),a=n.split(/\r?\n/).map(i=>i.trim()).filter(Boolean),r=[];for(const i of a){const o=normalizeDirectDomainInput(i);if(!o){e.classList.add("invalid"),t&&(t.textContent="请输入正确域名、URL、IP 或端口，例如 example.com、http://example.com:443、1.1.1.1:443"),e.title="错误内容："+i;return{ok:!1,value:n,error:i}}r.push(o)}e.classList.remove("invalid"),t&&(t.textContent=""),e.title="";return{ok:!0,value:Array.from(new Set(r)).join("\n")}}function normalizeDirectDomainInput(e){let t=String(e||"").trim().toLowerCase();if(!t||/\s/.test(t))return"";try{if(/^[a-z][a-z0-9+.-]*:\/\//i.test(t)){t=new URL(t).hostname}else{t=t.replace(/^\*\./,"").replace(/^\./,"");const cidrMatch=t.match(/^(\d{1,3}(?:\.\d{1,3}){3})\/(\d{1,2})$/);if(cidrMatch){const[,ip,mask]=cidrMatch;const m=Number(mask);if(m>=0&&m<=32&&ip.split(".").every(n=>Number(n)>=0&&Number(n)<=255))return `${ip}/${m}`}const n=t.match(/^\[([^\]]+)\](?::\d{1,5})?$/);if(n){t=n[1]}else{const a=t.split(/[/?#]/)[0];t=a.includes(":")&&a.indexOf(":")===a.lastIndexOf(":")?a.split(":")[0]:a}}}catch{return""}t=t.replace(/^\*\./,"").replace(/^\./,"").replace(/\.$/,"");return isValidDirectHost(t)?t:""}function isValidDirectHost(e){const t=String(e||"").trim().toLowerCase();if(!t||t.length>253||t.includes(".."))return!1;if(/^\d{1,3}(?:\.\d{1,3}){3}\/\d{1,2}$/.test(t)){const[ip,mask]=t.split("/");const m=Number(mask);return m>=0&&m<=32&&ip.split(".").every(n=>Number(n)>=0&&Number(n)<=255)}if(/^\d{1,3}(?:\.\d{1,3}){3}$/.test(t))return t.split(".").every(n=>Number(n)>=0&&Number(n)<=255);if(t.includes(":"))return /^[0-9a-f:]+$/i.test(t)&&(t.includes("::")||t.split(":").length===8);if(t==="localhost")return!0;if(!t.includes("."))return!1;return t.split(".").every(n=>/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(n))}function lt(){const e=Nt(),t=validateDirectDomains();return{mode:ke(s.selectedMode),systemProxy:c("systemProxy").checked,tun:c("tunProxy").checked,mixedPort:Number(c("mixedPort").value),proxyMap:c("proxyMap").checked,proxyMapStartPort:Number(c("proxyMapStartPort").value),proxyMapGroupIds:e,proxyMapGroupId:e[0]||"",appProxy:c("appProxy").checked,appProxyTargets:Kt(s.settings.appProxyTargets||[]),ruleSource:c("ruleSource").value,autoUpdateRules:!1,ruleUpdateInterval:"168h",fakeDns:!1,autoStart:!1,circuitBreaker:c("circuitBreaker")?c("circuitBreaker").checked:!0,autoFailover:Boolean(s.settings.autoFailover),theme:document.body.dataset.theme==="dark"?"dark":"light",directDomains:t.value}}function Zr(e){var r,i;const a=Nt();s.settings.proxyMapGroupIds=a,s.settings.proxyMapGroupId=a[0]||"",Gt(((r=s.lastState)==null?void 0:r.groups)||[],a),It(((i=s.lastState)==null?void 0:i.nodes)||[])}function saveProxyMapGroupsOnly(){var e,t;const n=((e=s.lastState)==null?void 0:e.settings)||s.settings||{},a=lt();a.proxyMap=Boolean(n.proxyMap),s.settings.proxyMap=a.proxyMap,c("proxyMap").checked=a.proxyMap,s.settingsVersion+=1,s.pendingSettings=a,s.settings=a,s.applyingSettings=!1,s.settingsSaveQueued=!0,s.settingsSaveRunning?clearTimeout(s.settingsTimer):tt(0),It(((t=s.lastState)==null?void 0:t.nodes)||[])}function Gt(e,t){const n=c("proxyMapGroupText");if(!t.length){n.textContent="请选择分组";return}if(t.length===1){const a=(e||[]).find(r=>r.id===t[0]);n.textContent=(a==null?void 0:a.name)||"1 个分组";return}n.textContent=String(t.length)+" 个分组"}function wt(e){c("proxyMapGroupMenu").classList.toggle("hidden",!e),c("proxyMapGroupToggle").setAttribute("aria-expanded",String(e))}function ue(e=500){const t=validateDirectDomains();if(!t.ok){clearTimeout(s.settingsTimer),P("直连域名格式错误");return}if(s.settingsVersion+=1,s.pendingSettings=lt(),s.settings=s.pendingSettings,s.applyingSettings=!1,s.settingsSaveQueued=!0,s.settingsSaveRunning){clearTimeout(s.settingsTimer);return}tt(e)}function tt(e=0){clearTimeout(s.settingsTimer),s.settingsTimer=window.setTimeout(()=>{Ur()},e)}async function Ur(){var a;if(s.settingsSaveRunning){s.settingsSaveQueued=!0;return}if(s.busyKey){tt(400);return}const e=s.pendingSettings||lt(),t=s.settingsVersion,n=((a=s.lastState)==null?void 0:a.settings)||s.settings||{};s.pendingSettings=e,s.settings=e,s.applyingSettings=!0,s.settingsSaveRunning=!0,s.settingsSaveQueued=!1,$e("settings","正在应用设置");try{await Pt(e),t===s.settingsVersion&&(s.settings=e,s.pendingSettings=null,P(qr(n,e)))}catch(r){if(await handleAdminRestartRequired(r,e)){s.pendingSettings=null}else P(J(r))}finally{if(s.settingsSaveRunning=!1,(t!==s.settingsVersion||s.settingsSaveQueued)&&s.pendingSettings){$e("",""),tt(0);return}s.applyingSettings=!1,s.pendingSettings=null,s.settingsSaveQueued=!1,$e("",""),await Me().catch(()=>{})}}function adminRestartCode(e){const t=J(e);return t.includes("ADMIN_RESTART_REQUIRED:TUN")?"tun":t.includes("ADMIN_RESTART_REQUIRED:NETWORK_TOOL")?"networkTool":""}async function handleAdminRestartRequired(e,t){const n=adminRestartCode(e);if(!n)return!1;if(n==="tun"){const a=await Dt({title:"需要管理员权限",message:"TUN 代理需要管理员权限",desc:"将保存当前设置，并以管理员身份重启 SanTian-Singbox。重启后会继续启用 TUN。",okText:"以管理员身份重启",cancelText:"取消"});if(!a){P("已取消开启 TUN 代理");return!0}await RestartTunAsAdminApi(t);return!0}const a=await Dt({title:"需要管理员权限",message:"网络修复工具需要管理员权限",desc:"将以管理员身份重启 SanTian-Singbox，重启后请重新点击该网络工具。",okText:"以管理员身份重启",cancelText:"取消"});if(!a){P("已取消管理员重启");return!0}await RestartNetworkToolAsAdminApi();return!0}function qr(e,t){const n=[],a=Boolean(e.systemProxy||e.tun||e.proxyMap||e.appProxy),r=Boolean(t.systemProxy||t.tun||t.proxyMap||t.appProxy);return!a&&r&&n.push("\u4EE3\u7406\u5DF2\u5F00\u542F"),a&&!r&&n.push("\u4EE3\u7406\u5DF2\u5173\u95ED"),ke(e.mode)!==ke(t.mode)&&n.push(t.mode==="global"?"已切换到全局模式":"已切换到规则模式"),xe(n,e.systemProxy,t.systemProxy,"系统代理"),xe(n,e.tun,t.tun,"TUN 代理"),xe(n,e.proxyMap,t.proxyMap,"代理映射"),xe(n,e.appProxy,t.appProxy,"全局代理应用"),xe(n,e.autoStart!==!1,t.autoStart!==!1,"开机自启动"),xe(n,e.circuitBreaker!==!1,t.circuitBreaker!==!1,"智能分流熔断"),xe(n,Boolean(e.autoFailover),Boolean(t.autoFailover),"自动切换"),ge(n,e.mixedPort,t.mixedPort,"\u672C\u5730\u7AEF\u53E3\u5DF2\u66F4\u65B0"),ge(n,e.proxyMapStartPort,t.proxyMapStartPort,"\u4EE3\u7406\u6620\u5C04\u7AEF\u53E3\u5DF2\u66F4\u65B0"),ge(n,Ze(e).join(","),Ze(t).join(","),"\u4EE3\u7406\u6620\u5C04\u5206\u7EC4\u5DF2\u66F4\u65B0"),ge(n,kt(e.appProxyTargets),kt(t.appProxyTargets),"\u5168\u5C40\u6D41\u91CF\u5217\u8868\u5DF2\u66F4\u65B0"),ge(n,e.ruleSource,t.ruleSource,"规则源已更新"),ge(n,e.directDomains||"",t.directDomains||"","\u76F4\u8FDE\u57DF\u540D\u5DF2\u66F4\u65B0"),n[0]||"\u8FD0\u884C\u8BBE\u7F6E\u5DF2\u66F4\u65B0"}function xe(e,t,n,a){Boolean(t)!==Boolean(n)&&e.push(a+"已更新")}function ge(e,t,n,a){String(t!=null?t:"")!==String(n!=null?n:"")&&e.push(a)}function kt(e=[]){return(e||[]).map(t=>String(t.type)+":"+String(t.path)+":"+String(t.enabled!==!1)).join("|")}function Kt(e=[]){const t=new Set;return(e||[]).filter(n=>{if(!(n!=null&&n.path)||n.type!=="app"&&n.type!=="folder")return!1;const a=n.type+":"+String(n.path).replaceAll("/","\\").toLowerCase();return t.has(a)?!1:(t.add(a),!0)})}function we(e,t="change",n=0){const a=c(e);!a||a.addEventListener(t,()=>{ue(n)})}async function Mt(e){await H(e==="folder"?"pickAppProxyFolder":"pickAppProxyApp","正在选择目标",async()=>{const t=await hn(e);if(!(t!=null&&t.path))return;const n=s.settings.appProxyTargets||[];if(n.some(r=>r.id===t.id||r.type===t.type&&Gr(r.path,t.path))){P("该目标已在列表中");return}s.settings.appProxyTargets=[...n,t],c("appProxy").checked=!0,Ke(s.settings.appProxyTargets);const a=lt();await Pt(a),s.settings=a,P("\u5168\u5C40\u6D41\u91CF\u5217\u8868\u5DF2\u66F4\u65B0")})}function Gr(e,t){return String(e||"").replaceAll("/","\\").toLowerCase()===String(t||"").replaceAll("/","\\").toLowerCase()}async function runNetworkTool(e){await H("networkTool:"+e,"正在运行网络工具",async()=>{try{const t=await RunNetworkToolApi(e);if(e==="network_reset"){P(t.success?"已打开 Windows 网络重置设置页":"网络重置页面打开失败");return}showNetworkToolResult(t),P(t.success?"网络工具运行完成":"网络工具运行失败")}catch(t){if(!await handleAdminRestartRequired(t,null))throw t}},null,{skipRefresh:!0})}function showNetworkToolResult(e={}){c("networkToolTitle").textContent=e.name||"工具运行结果",c("networkToolCommand").textContent="命令："+(e.command||""),c("networkToolOutput").textContent=e.output||"无输出",c("networkToolDialog").classList.remove("hidden")}async function updateRulesInBackground(){if(s.updatingRules)return;s.updatingRules=!0;P("正在后台更新规则");try{const e=await UpdateRulesApi(),t=e.count||"0";P(e.status==="latest"?"规则已是最新版本"+(e.version?"："+e.version:""):"规则已后台更新："+t+" 个文件，重启代理后生效")}catch(e){P("规则更新失败："+J(e))}finally{s.updatingRules=!1,Me().catch(()=>{})}}async function updateCoreInBackground(e=!1){if(s.updatingCore)return;s.updatingCore=!0,Ge(s.lastState||{}),P("正在后台检测与下载 sing-box 内核...");try{const t=await Rn(),n=t.version||"最新版本";P(t.status==="already_latest"?"sing-box 已是最新版本："+n:"sing-box 已更新到 "+n),e&&updateRulesInBackground()}catch(t){P("sing-box 内核更新失败："+J(t))}finally{s.updatingCore=!1,Ge(s.lastState||{})}}function Kr(){document.querySelectorAll(".menu").forEach(e=>{e.addEventListener("click",()=>Tt(e.dataset.page))}),c("logPreview").addEventListener("click",()=>Tt("logs")),document.querySelectorAll(".mode").forEach(e=>{e.addEventListener("click",()=>{s.selectedMode=e.dataset.mode,document.querySelectorAll(".mode").forEach(t=>t.classList.remove("active")),e.classList.add("active"),updateDirectDomainsVisibility(),ue(250)})}),c("tunProxy").addEventListener("change",()=>{c("tunProxy").checked&&(c("systemProxy").checked=!1),ue(250)}),c("systemProxy").addEventListener("change",()=>{c("systemProxy").checked&&(c("tunProxy").checked=!1),ue(250)}),c("proxyMap").addEventListener("change",()=>{ue(0)}),c("appProxy").addEventListener("change",()=>{Ke(s.settings.appProxyTargets||[]),ue(0)}),we("ruleSource"),we("autoStart"),we("circuitBreaker"),we("mixedPort","input",700),we("proxyMapStartPort","input",700),c("pickAppProxyFolder").addEventListener("click",()=>Mt("folder")),c("pickAppProxyApp").addEventListener("click",()=>Mt("app")),c("proxyMapGroupToggle").addEventListener("click",e=>{e.stopPropagation(),wt(c("proxyMapGroupMenu").classList.contains("hidden"))}),c("proxyMapGroupSelect").addEventListener("click",e=>{e.stopPropagation()}),document.addEventListener("click",()=>{wt(!1)}),c("directDomains").addEventListener("input",()=>{validateDirectDomains(),ue(700)}),c("copyProxyMapAll").addEventListener("click",()=>H("copyProxyMapAll","正在复制代理映射",async()=>{var t;const e=sr(((t=s.lastState)==null?void 0:t.nodes)||[]);if(!e){P("暂无可复制的代理映射");return}await Le(e),P("\u4EE3\u7406\u6620\u5C04\u5DF2\u590D\u5236")})),c("updateApp") && c("updateApp").addEventListener("click", () => checkAndPromptAppUpdate(true));
    c("updateCore").addEventListener("click",async()=>{const e=await showUpdateChoiceDialog();if(!e)return;if(e==="app"){checkAndPromptAppUpdate(true);return}if(e==="rules"){updateRulesInBackground();return}if(e==="core"){updateCoreInBackground(!1);return}if(e==="all"){updateCoreInBackground(!0);return}}),c("restartProxy").addEventListener("click",()=>H("restartProxy","正在重启代理",async()=>{await mn(),P("\u4EE3\u7406\u5DF2\u81EA\u68C0\u5E76\u91CD\u542F")})),c("themeToggle").addEventListener("click",toggleTheme),c("nodeInput").addEventListener("input",_e);const runNodeSearch=()=>{var e,t;s.nodeSearchQuery=String(c("nodeSearchInput").value||"").trim().toLowerCase(),Ce(((e=s.lastState)==null?void 0:e.nodes)||[],((t=s.lastState)==null?void 0:t.selectedId)||"")},clearNodeSearchIfEmpty=()=>{var e,t;String(c("nodeSearchInput").value||"").trim()||!s.nodeSearchQuery||(s.nodeSearchQuery="",Ce(((e=s.lastState)==null?void 0:e.nodes)||[],((t=s.lastState)==null?void 0:t.selectedId)||""))};c("nodeSearchBtn").addEventListener("click",runNodeSearch);const updateAutoFailoverUI=(af)=>{const sw=c("autoFailoverSwitch");if(sw)sw.checked=af;const ctrl=c("autoFailoverControl");if(ctrl){ctrl.classList.toggle("active",af),ctrl.title=af?"已开启自动切换：节点断网时将自动切换至同分组最低延迟节点（点击关闭）":"已关闭自动切换：仅手动切换节点（点击开启）"}const b=c("autoFailoverBtn");if(b){b.classList.toggle("active",af),b.textContent=af?"自动切换：开":"自动切换：关",b.title=af?"已开启自动切换：节点断网时将自动切换至同分组最低延迟节点（点击关闭）":"已关闭自动切换：仅手动切换节点（点击开启）"}};const toggleAutoFailover=(val)=>{s.settings.autoFailover=typeof val==="boolean"?val:!s.settings.autoFailover;const af=Boolean(s.settings.autoFailover);updateAutoFailoverUI(af),ue(0),P(af?"已开启节点断网自动切换（同分组）":"已关闭节点自动切换（仅手动切换）")};c("autoFailoverSwitch")&&c("autoFailoverSwitch").addEventListener("change",e=>toggleAutoFailover(e.target.checked));c("autoFailoverBtn")&&c("autoFailoverBtn").addEventListener("click",()=>toggleAutoFailover());c("nodeSearchInput").addEventListener("input",clearNodeSearchIfEmpty),c("nodeSearchInput").addEventListener("keydown",e=>{e.key==="Enter"&&(e.preventDefault(),runNodeSearch())}),c("addGroupBtn").addEventListener("click",()=>{s.deletingGroupMode=!1,Ot({title:"添加分组",message:"每个组管理不同的节点",okText:"添加"}).then(e=>{!e||H("createGroup","正在添加分组",async()=>{const t=await pn(e);s.activeGroupId=t.id||s.activeGroupId,P("节点组已添加")})})}),c("deleteGroupModeBtn").addEventListener("click",()=>{var e,t;s.deletingGroupMode=!s.deletingGroupMode,Oe(((e=s.lastState)==null?void 0:e.groups)||[],((t=s.lastState)==null?void 0:t.nodes)||[]),P(s.deletingGroupMode?"\u8BF7\u9009\u62E9\u8981\u5220\u9664\u7684\u8282\u70B9\u7EC4":"\u5DF2\u9000\u51FA\u5220\u9664\u5206\u7EC4")}),c("exportGroupLinksBtn").addEventListener("click",()=>H("exportGroupLinks","正在导出当前分组",async()=>{if(!await Tr()){P("\u5DF2\u53D6\u6D88\u5BFC\u51FA");return}P("\u5F53\u524D\u5206\u7EC4\u94FE\u63A5\u5DF2\u5BFC\u51FA")},null,{skipRefresh:!0})),c("importBtn").addEventListener("click",()=>H("import","正在导入节点",async()=>{const e=c("nodeInput").value.trim();if(!e){P("\u8BF7\u5148\u7C98\u8D34\u8282\u70B9\u6216\u8BA2\u9605\u94FE\u63A5");return}if(pt(e)==="subscription"){const n=sa(e),a=await gn(n);c("nodeInput").value="",_e(),P("订阅已导入："+a.groupName+"，节点 "+a.added+" 个");return}const t=s.activeGroupId||"default";let n=await fn(e,t,!1);if(n.requiresConfirmation){const a=Number(n.duplicateCount||0),r=await Dt({title:"发现重复节点",message:`检测到 ${a} 个重复节点，是否仍然导入？`,desc:"重复节点可能位于当前分组或其他分组。取消后本次不导入任何节点。",okText:"确认导入",cancelText:"取消"});if(!r){P("已取消导入重复节点");return}n=await fn(e,t,!0)}c("nodeInput").value="",_e();const a=Number(n.duplicateCount||0);P("已导入 "+n.added+" 个节点到当前分组"+(a>0?"，其中重复 "+a+" 个":""))})),c("refreshConnections").addEventListener("click",()=>H("refreshConnections","正在刷新连接",async()=>{await Me()},null,{skipRefresh:!0})),c("connectionSearch").addEventListener("input",e=>{var t;s.connectionQuery=e.target.value,Be(((t=s.lastState)==null?void 0:t.logs)||[])}),c("connectionType").addEventListener("change",e=>{var t;s.connectionType=e.target.value,Be(((t=s.lastState)==null?void 0:t.logs)||[])}),c("connectionRule").addEventListener("change",e=>{var t;s.connectionRule=e.target.value,Be(((t=s.lastState)==null?void 0:t.logs)||[])}),c("logSearch").addEventListener("input",e=>{s.logQuery=e.target.value,s.fullLogs?Se(s.fullLogs):loadFullLogs()}),c("logLevel").addEventListener("change",e=>{s.logLevel=e.target.value,s.fullLogs?Se(s.fullLogs):loadFullLogs()}),c("logAutoScroll").addEventListener("change",e=>{s.logAutoScroll=e.target.checked,s.fullLogs?Se(s.fullLogs):loadFullLogs()}),c("clearLogView").addEventListener("click",()=>{const e=s.fullLogs||((s.lastState||{}).logs)||[];s.clearedLogIndex=e.length,Se(e),P("\u5F53\u524D\u65E5\u5FD7\u89C6\u56FE\u5DF2\u6E05\u7A7A")}),c("copyLogs").addEventListener("click",()=>H("copyLogs","正在复制日志",async()=>{const e=yt();if(!e){P("没有可复制的日志");return}await Le(e),P("\u65E5\u5FD7\u5DF2\u590D\u5236")})),c("exportLogs").addEventListener("click",()=>{const e=yt();if(!e){P("没有可导出的日志");return}nr("SanTian-Singbox-logs-"+rr()+".log",e),P("\u65E5\u5FD7\u5DF2\u5BFC\u51FA")}),c("logToTop").addEventListener("click",()=>{c("logItems").scrollTop=0}),c("logToBottom").addEventListener("click",()=>{const e=c("logItems");e.scrollTop=e.scrollHeight}),document.querySelectorAll("[data-tester-tab]").forEach(e=>{e.addEventListener("click",()=>Fr(e.dataset.testerTab))}),c("speedTrafficMB").addEventListener("change",()=>{c("speedTrafficMB").value=String(Ye().trafficMB)}),c("speedDurationSec").addEventListener("change",()=>{c("speedDurationSec").value=String(Ye().durationSec)}),c("batchNetworkMode")&&c("batchNetworkMode").addEventListener("change",()=>{}),c("pasteTestNodes").addEventListener("click",()=>H("pasteTestNodes","正在读取剪贴板",async()=>{const e=await dn();if(!e.trim()){P("\u526A\u8D34\u677F\u6CA1\u6709\u8282\u70B9\u5185\u5BB9");return}c("testInput").value=e,await Vt(),P("\u5DF2\u4ECE\u526A\u8D34\u677F\u8BFB\u53D6")})),c("batchDelayTest").addEventListener("click",()=>Jr()),c("stopBatchTest").addEventListener("click",ea),c("copyProxyMapSocks5")&&c("copyProxyMapSocks5").addEventListener("click",copyProxyMapSocks5),c("exportProxyMapCsv")&&c("exportProxyMapCsv").addEventListener("click",exportProxyMapCsv),c("filterDeadNodes")&&c("filterDeadNodes").addEventListener("click",filterDeadNodes),c("copyWorkingNodes")&&c("copyWorkingNodes").addEventListener("click",copyWorkingNodes),c("saveBatchToGroup")&&c("saveBatchToGroup").addEventListener("click",saveBatchToGroup),c("clearTestNodes").addEventListener("click",()=>{s.testRunId+=1,s.testNodes=[],s.batchTesting=!1,s.ipBackfilling=!1,s.ipBackfillRunId=0,s.testStopRequested=!0,s.testBusyKey="",s.testBusyKeys.clear(),se(),P("\u6D4B\u8BD5\u8282\u70B9\u5217\u8868\u5DF2\u6E05\u7A7A")}),c("startMTR").addEventListener("click",nt),c("stopMTR").addEventListener("click",Xr),c("mtrTarget").addEventListener("keydown",e=>{e.key==="Enter"&&nt()}),c("clearMTROutput").addEventListener("click",()=>{s.mtrOutput=[],ot()}),c("startIperfUpload").addEventListener("click",()=>St("upload")),c("startIperfDownload").addEventListener("click",()=>St("download")),c("stopIperf").addEventListener("click",Wr),c("clearIperfOutput").addEventListener("click",()=>{s.iperfOutput=[],st()}),c("flushDnsCacheTool").addEventListener("click",()=>runNetworkTool("flush_dns")),c("networkToolClose").addEventListener("click",()=>c("networkToolDialog").classList.add("hidden")),c("networkToolDialog").addEventListener("click",e=>{})}function Fr(e){s.testerTab=["batch","mtr","iperf3","localSpeedtest","toolbox"].includes(e)?e:"batch",document.querySelectorAll("[data-tester-tab]").forEach(t=>{t.classList.toggle("active",t.dataset.testerTab===s.testerTab)}),c("batchTesterTab").classList.toggle("hidden",s.testerTab!=="batch"),c("mtrTesterTab").classList.toggle("hidden",s.testerTab!=="mtr"),c("iperfTesterTab").classList.toggle("hidden",s.testerTab!=="iperf3"),c("localSpeedtestTab")&&c("localSpeedtestTab").classList.toggle("hidden",s.testerTab!=="localSpeedtest"),c("toolboxTesterTab")&&c("toolboxTesterTab").classList.toggle("hidden",s.testerTab!=="toolbox"),(s.testerTab==="mtr"||s.testerTab==="iperf3"||s.testerTab==="localSpeedtest")&&Qr()}function Tt(e){const t=["home","proxyMap","connections","logs","tester"];if(s.page=t.includes(e)?e:"home",c("homePage").classList.toggle("hidden",s.page!=="home"),c("proxyMapPage").classList.toggle("hidden",s.page!=="proxyMap"),c("connectionsPage").classList.toggle("hidden",s.page!=="connections"),c("logsPage").classList.toggle("hidden",s.page!=="logs"),c("testerPage").classList.toggle("hidden",s.page!=="tester"),document.querySelectorAll(".menu").forEach(n=>{n.classList.toggle("active",n.dataset.page===s.page)}),refreshVisibleStatePanels(s.lastState||{}),s.page==="logs"&&s.logAutoScroll){const n=c("logItems");n.scrollTop=n.scrollHeight}s.page==="tester"&&We()}async function Ft(){try{await Me()}catch(e){if(!Fe()){Ge({running:!1,appVersion:"0.0.1",coreVersion:"内核未知",settings:{},rulePresets:[],nodes:[],groups:[],logs:[],configMode:{}});return}P(J(e))}}function queueStateRefresh(e=260){if(s.uiPaused||document.hidden)return;clearTimeout(window.__sanTianStateRefreshTimer),window.__sanTianStateRefreshTimer=setTimeout(()=>Me().catch(()=>{}),e)}function jr(){!Fe()||(Te("state-changed",()=>queueStateRefresh()),Te("system-suspend",handleSystemSuspend),Te("system-resume",handleSystemResume),Te("speed-progress",Hr),Te("iperf-output",jt),Te("mtr-output",Ht),Te("auto-failover-switched",e=>{e&&e.to&&P(`【自动切换】原节点断网，已自动切换至同分组最低延迟节点：${e.to}${e.delayMs?` (${e.delayMs}ms)`:""}`),queueStateRefresh(50)}))}function handleSystemSuspend(){s.uiPaused=!0,window.__sanTianStateQueued=!1,clearTimeout(window.__sanTianStateRefreshTimer),clearTimeout(s.resumeTimer)}function handleSystemResume(){s.uiPaused=!1,window.__sanTianStateQueued=!1,clearTimeout(s.resumeTimer),s.proxyMapScrollTop=0,s.resumeTimer=setTimeout(()=>Ft(),1200)}function scheduleStatePoll(){const e=Date.now();if(!s.uiPaused&&!document.hidden){if(s.lastStatePollAt&&e-s.lastStatePollAt>9e4){handleSystemResume();return}Ft()}}function Hr(e){if(!(e!=null&&e.raw)||s.testStopRequested)return;const t=s.testNodes.findIndex(a=>a.raw===e.raw);if(t<0)return;const n=s.testNodes[t];n.speedRunId===s.testRunId&&(e.downloadMbps>0&&(n.downloadMbps=Number(e.downloadMbps||0)),e.uploadMbps>0&&(n.uploadMbps=Number(e.uploadMbps||0)),n.status=e.status||dt("speed"),n.error="",se())}function Fe(){var e,t;return Boolean(window.runtime&&((t=(e=window.go)==null?void 0:e.main)==null?void 0:t.App))}function jt(e){const t=String(e||"").trimEnd();!t||(s.iperfOutput.push(t),s.iperfOutput.length>500&&(s.iperfOutput=s.iperfOutput.slice(-500)),t.includes("iperf3 已结束")&&(s.iperfRunning=!1,te()),st())}function Ht(e){const t=String(e||"").trimEnd();!t||(s.mtrOutput.push(t),s.mtrOutput.length>600&&(s.mtrOutput=s.mtrOutput.slice(-600)),t.includes("MTR 已结束")&&(s.mtrRunning=!1,te()),ot())}async function Qr(){if(!Fe()){s.toolStatus={nexttrace:{ready:!1,errorText:"\u6D4F\u89C8\u5668\u9884\u89C8\u4E0D\u53EF\u7528"},iperf3:{ready:!1,errorText:"\u6D4F\u89C8\u5668\u9884\u89C8\u4E0D\u53EF\u7528"}},We();return}try{s.toolStatus=await Pn()}catch(e){s.toolStatus={nexttrace:{ready:!1,errorText:J(e)},iperf3:{ready:!1,errorText:J(e)}}}We()}async function St(e){if(s.iperfRunning)return;const t=c("iperfTarget").value.trim(),n=Number(c("iperfParallel").value||8);if(!t){P("请输入 iperf3 服务器地址");return}const nm=c("iperfNetworkMode")?c("iperfNetworkMode").value:"direct";s.iperfRunning=!0,s.iperfOutput=[],st(),te();try{const a=await bn({target:t,parallel:n,mode:e,networkMode:nm});jt("命令："+(a.command||"")),P(e==="download"?"iperf3 \u4E0B\u8F7D\u6D4B\u8BD5\u5DF2\u5F00\u59CB":"iperf3 \u4E0A\u4F20\u6D4B\u8BD5\u5DF2\u5F00\u59CB")}catch(a){s.iperfRunning=!1,P(J(a))}finally{te()}}async function Wr(){if(!!s.iperfRunning)try{await kn()}catch(e){P(J(e))}}async function nt(){if(s.mtrRunning)return;const e=c("mtrTarget").value.trim();if(!e){P("请输入 MTR 目标");return}s.mtrRunning=!0,s.mtrOutput=[],ot(),ne(),te();try{const n=c("mtrNetworkMode")?c("mtrNetworkMode").value:"local",pv=c("mtrProtocol")?c("mtrProtocol").value:"icmp";let pr="icmp",pt=0;pv==="tcp_443"?(pr="tcp",pt=443):pv==="tcp_80"&&(pr="tcp",pt=80);const t=await xn({target:e,networkMode:n,protocol:pr,port:pt});Ht("命令："+(t.command||"")),P(n==="proxy"?"代理测试 MTR 已开始":"本地测试 MTR 已开始")}catch(t){s.mtrRunning=!1,P(J(t))}finally{ne(),te()}}async function Xr(){if(!!s.mtrRunning)try{await Mn()}catch(e){P(J(e))}}async function Vt(){const e=c("testInput").value.trim();if(!e)return P("请先输入节点"),!1;const t=await yn(e);return s.testRunId+=1,s.testStopRequested=!1,s.batchTesting=!1,s.ipBackfilling=!1,s.ipBackfillRunId=0,s.testBusyKey="",s.testBusyKeys.clear(),s.testNodes=(t.nodes||[]).map(n=>({...n,pingAddress:n.address||"",country:n.country||tn(n),pingMs:0,urlDelayMs:0,outboundIp:"",downloadMbps:0,uploadMbps:0,error:"",status:"待测试"})),se(),P("已解析 "+s.testNodes.length+" 个节点"),!0}function Jr(){s.batchTesting||(s.testStopRequested=!1,Qt().catch(e=>{s.batchTesting=!1,s.ipBackfilling=!1,ne(),se(),P(J(e))}))}async function Qt(){if(!await Vt()||!s.testNodes.length)return;const testOptions=await prepareBatchRawTest(s.testNodes);if(!testOptions)return;const delayMetric=await showDelayMetricDialog({title:"选择批量测试方式",message:"请选择本次批量测试的延迟类型。"});if(!delayMetric)return;testOptions.delayMetric=delayMetric;const t=s.testRunId;s.currentRawTestOptions=testOptions,s.batchTesting=!0,s.testBusyKeys.clear(),ne(),se();let n=0;const a=async()=>{const i=testOptions.delayMetric==="real"?"url":"ping";for(;!qe(t)&&n<s.testNodes.length;){const o=n;n+=1,await Ue(o,i,!0,t)}},r=Math.min(Et,s.testNodes.length);if(await Promise.all(Array.from({length:r},a)),!!ye(t)){if(s.batchTesting=!1,ne(),se(),s.testStopRequested){Yt(),P("\u8282\u70B9\u6D4B\u8BD5\u5DF2\u505C\u6B62");return}const i=s.testNodes.some(o=>o.urlDelayMs>0||o.pingMs>0);if(!i){P("节点测试已完成，失败节点已跳过出站 IP 回填");return}P((testOptions.delayMetric==="real"?"真连接延迟":"TCP延迟")+"完成，出站 IP 后台回填"),Yr(t)}}function Yr(e){s.ipBackfilling=!0,s.ipBackfillRunId=e,ne(),se(),zr(e).catch(t=>{ye(e)&&!s.testStopRequested&&P(J(t))}).finally(()=>{!ye(e)||s.ipBackfillRunId!==e||(s.ipBackfilling=!1,s.ipBackfillRunId=0,s.testBusyKey="",ne(),se())})}async function zr(e){let t=0;const n=async()=>{for(;!qe(e)&&t<s.testNodes.length;){const r=t;t+=1;const i=s.testNodes[r];if(!i||i.urlDelayMs<=0&&i.pingMs<=0){continue}await Ue(r,"ip",!0,e)}},a=Math.min(Et,s.testNodes.length);await Promise.all(Array.from({length:a},n))}async function Ue(e,t,n=!1,a=s.testRunId){var C,N;const r=s.testNodes[e];if(!r||!ye(a)||t!=="speed"&&qe(a))return;const i=r.raw,o=Wt(e,a),u=Xt(e,t,a),b=Ye(),S=t!=="ip";S&&(s.testBusyKeys.add(o),s.testBusyKey=u),ne(),S&&(r.error=""),t==="speed"&&(r.speedRunId=a,r.downloadMbps=0,r.uploadMbps=0),S&&(r.status=t==="speed"?"正在测速："+b.trafficMB+"M / "+b.durationSec+"s":dt(t)),se();try{const O=await $n(r.raw,t,{...b,...rawTestOptionsForNode(r)});if(!ye(a)||((C=s.testNodes[e])==null?void 0:C.raw)!==i||t==="ip"&&s.testStopRequested)return;t==="ping"&&(r.pingAddress=O.pingAddress||r.address||"",r.pingMs=O.pingMs||O.delayMs||0,r.status="Ping "+r.pingMs+" ms"),t==="url"&&(r.urlDelayMs=O.urlDelayMs||O.delayMs||0,r.status="URL "+r.urlDelayMs+" ms"),t==="delay"&&(r.urlDelayMs=O.urlDelayMs||O.delayMs||0,r.status="URL "+r.urlDelayMs+" ms"),t==="ip"&&(r.outboundIp=O.outboundIp||"",r.country=O.country||tn({...r,address:r.outboundIp})||r.country,r.status=r.outboundIp?"出口 IP "+r.outboundIp:"出口 IP 未知"),t==="speed"&&(r.downloadMbps=Number(O.downloadMbps||0),r.uploadMbps=Number(O.uploadMbps||0),r.status="下载 "+r.downloadMbps.toFixed(2)+" Mbps / 上传 "+r.uploadMbps.toFixed(2)+" Mbps"),n||P(r.status)}catch(O){if(!ye(a)||((N=s.testNodes[e])==null?void 0:N.raw)!==i||t==="ip"&&s.testStopRequested)return;if(t==="ping"&&(r.pingMs=-1),(t==="url"||t==="delay")&&(r.urlDelayMs=-1),t==="ip"){r.outboundIp="未知",r.status="出口 IP 失败",n||P($t(t));return}t==="speed"&&(r.downloadMbps=-1,r.uploadMbps=-1),r.error=J(O),r.status=en(r.error),n||P($t(t))}finally{S&&(s.testBusyKeys.delete(o),s.testBusyKey===u&&(s.testBusyKey="")),ne(),se()}}function Wt(e,t=s.testRunId){return String(t)+":"+String(e)}function Xt(e,t,n=s.testRunId){return String(n)+":"+String(e)+":"+String(t)}function ye(e){return!e||e===s.testRunId}function qe(e){return!ye(e)||s.testStopRequested}function Jt(){return Boolean(s.batchTesting||s.ipBackfilling||s.testBusyKey||s.testBusyKeys.size)}function ea(){!Jt()||(s.testRunId+=1,s.testStopRequested=!0,s.batchTesting=!1,s.ipBackfilling=!1,s.ipBackfillRunId=0,s.testBusyKey="",s.testBusyKeys.clear(),Yt(),ne(),se(),P("\u5DF2\u505C\u6B62\u6392\u961F\u4EFB\u52A1"))}function Yt(){s.testNodes.forEach(e=>{!e.pingMs&&!e.urlDelayMs&&!e.outboundIp&&(e.status="\u5DF2\u505C\u6B62")})}function $t(e){return e==="speed"?"\u6D4B\u901F\u5931\u8D25":e==="ping"?"Ping 测试失败":e==="ip"?"出站 IP 测试失败":"延迟测试失败"}function preserveNodeScroll(){const e=c("nodes"),t=String(s.nodeSearchQuery||"").trim().toLowerCase()?"__search__":s.activeGroupId||"default",n=Math.max(0,Number(e?e.scrollTop:0)||Number(s.nodeScrollTopByGroup[t]||0));return()=>{const a=c("nodes");if(!a)return;s.nodeScrollTopByGroup[t]=n;const r=()=>{a.scrollTop=Math.min(n,Math.max(0,a.scrollHeight-a.clientHeight))};r(),requestAnimationFrame(r)}}async function H(e,t,n,a,r={}){if(!s.busyKey){const i=r.preserveNodeScroll?preserveNodeScroll():null;$e(e,t),i&&i();try{await n()}catch(o){P(J(o))}finally{a&&a(),$e("",""),i&&i(),r.skipRefresh||await Me().catch(()=>{}),i&&i()}}}function ta(e){return s.testingNodeId===e.id?"\u6D4B\u8BD5\u4E2D":e.delayMs>0?String(e.delayMs)+" ms":e.delayMs<0?"失败":"\u672A\u6D4B\u901F"}function Ne(e){const t=Number(e||0);if(!Number.isFinite(t)||t<=0)return"0 B";const n=["B","KB","MB","GB","TB"];let a=t,r=0;for(;a>=1024&&r<n.length-1;)a/=1024,r+=1;const i=r===0||a>=100?0:a>=10?1:2;return a.toFixed(i)+" "+n[r]}function zt(e){return Number((e==null?void 0:e.uploadTotal)||0)+Number((e==null?void 0:e.downloadTotal)||0)}function Ie(e,t){if(e>0){const n=t==="Mbps"?Number(e).toFixed(2):Math.round(e);return String(n)+" "+t}return e<0?"失败":"未测"}function Ae(e){return e<0?"metric bad":e>0?"metric ok":"metric"}function na(e){return e==="失败"?"失败":e||"未测"}function ra(e){return e==="失败"?"metric bad":e?"metric ok":"metric"}function dt(e){return e==="ping"?"正在测试 Ping 延迟":e==="url"||e==="delay"?"正在测试 URL 延迟":e==="ip"?"正在测试出站 IP":e==="speed"?"\u6B63\u5728\u6D4B\u901F\uFF1A"+s.speedTrafficMB+"M / "+s.speedDurationSec+"s":"正在测试"}function aa(e,t){return t?"\u6D4B\u8BD5\u4E2D":e.error?e.status||en(e.error):e.urlDelayMs>0?"正常":e.urlDelayMs<0?"异常":"未测"}function en(e){const t=String(e||"测试失败").replace(/^Error:\s*/i,"").trim();return t.length>32?t.slice(0,32)+"...":t}function tn(e){if(e.country)return e.country;const t=((e.name||"")+" "+(e.address||"")).toLowerCase(),n=[["\u4e2d\u56fd\u9999\u6e2f",["\u9999\u6e2f","\u6e2f","hk","hong kong"]],["\u4e2d\u56fd\u53f0\u6e7e",["\u53f0\u6e7e","\u53f0","tw","taiwan"]],["\u65e5\u672c",["\u65e5\u672c","\u65e5","jp","japan","tokyo","osaka"]],["\u65b0\u52a0\u5761",["\u65b0\u52a0\u5761","\u72ee\u57ce","sg","singapore"]],["\u7f8e\u56fd",["\u7f8e\u56fd","\u7f8e","us","usa","united states","los angeles","san jose"]],["\u97e9\u56fd",["\u97e9\u56fd","\u97e9","kr","korea","seoul"]],["\u82f1\u56fd",["\u82f1\u56fd","\u82f1","uk","gb","united kingdom","london"]],["\u5fb7\u56fd",["\u5fb7\u56fd","\u5fb7","de","germany"]],["\u6cd5\u56fd",["\u6cd5\u56fd","\u6cd5","fr","france"]],["\u52a0\u62ff\u5927",["\u52a0\u62ff\u5927","ca","canada"]],["\u6fb3\u5927\u5229\u4e9a",["\u6fb3\u5927\u5229\u4e9a","\u6fb3\u6d32","au","australia"]],["\u5370\u5ea6",["\u5370\u5ea6","in","india"]],["\u4fc4\u7f57\u65af",["\u4fc4\u7f57\u65af","\u4fc4","ru","russia"]]];for(const[a,r]of n)if(r.some(i=>t.includes(i)))return a;return"未知"}function P(e){const t=c("toast");t.innerHTML='<span class="toast-icon" aria-hidden="true">OK</span><span>'+m(e)+"</span>",t.classList.add("show"),clearTimeout(window.__toastTimer);const n=String(e).length>24?6e3:2600;window.__toastTimer=setTimeout(()=>t.classList.remove("show"),n)}function J(e){return e!=null&&e.message?e.message:String(e||"操作失败")}function m(e){return String(e!=null?e:"").replaceAll("&","&amp;").replaceAll("<","&lt;").replaceAll(">","&gt;").replaceAll('"',"&quot;")}function nn(e){return String(e||"").replaceAll("\r",`
`).replaceAll("	",`
`).split(`
`).map(t=>t.trim()).filter(Boolean)}function sa(e){return nn(e)[0]||""}function pt(e){const t=nn(e);return t.length===1&&oa(t[0])?"subscription":"nodes"}function oa(e){try{const t=new URL(e);return t.protocol!=="http:"&&t.protocol!=="https:"||t.username||t.password?!1:!(t.port&&t.pathname==="/"&&!t.search&&!t.hash)}catch{return!1}}function _e(){if(W("import")){te();return}c("importBtn").textContent=rn()}function rn(){const e=c("nodeInput").value.trim();return e?pt(e)==="subscription"?"导入订阅":"导入节点":"导入"}
let appUpdateDismissedThisSession = false;

function showAppUpdateDialog(info) {
  const dlg = c("appUpdateDialog");
  if (!dlg) return;
  const verText = c("appUpdateVersionText");
  const notes = c("appUpdateNotes");
  const progressBox = c("appUpdateProgressBox");
  const progressBar = c("appUpdateProgressBar");
  const statusText = c("appUpdateStatusText");
  const percentText = c("appUpdatePercentText");
  const btnCancel = c("appUpdateCancel");
  const btnConfirm = c("appUpdateConfirm");
  const btnClose = c("appUpdateClose");

  const curVer = info.currentVersion || (s.lastState && s.lastState.appVersion) || "";
  const newVer = info.latestVersion || "";
  if (verText) {
    verText.textContent = `当前版本: v${curVer.replace(/^v/, "") || "未知"} → 最新版本: v${newVer.replace(/^v/, "") || "最新"}`;
  }
  if (notes) {
    notes.textContent = info.releaseNotes ? info.releaseNotes.trim() : "暂无详细更新说明";
  }

  if (progressBox) progressBox.classList.add("hidden");
  if (progressBar) progressBar.style.width = "0%";
  if (statusText) statusText.textContent = "准备中...";
  if (percentText) percentText.textContent = "0%";
  if (btnCancel) {
    btnCancel.disabled = false;
    btnCancel.classList.remove("hidden");
    btnCancel.textContent = "取消";
  }
  if (btnConfirm) {
    btnConfirm.disabled = false;
    btnConfirm.textContent = "立即更新";
  }
  if (btnClose) btnClose.disabled = false;

  dlg.classList.remove("hidden");
  if (btnConfirm) btnConfirm.focus();

  const closeDialog = (userDismissed = false) => {
    dlg.classList.add("hidden");
    if (userDismissed) {
      appUpdateDismissedThisSession = true;
    }
  };

  if (btnCancel) btnCancel.onclick = () => closeDialog(true);
  if (btnClose) btnClose.onclick = () => closeDialog(true);

  const onKey = (ev) => {
    if (ev.key === "Escape" && (!btnConfirm || !btnConfirm.disabled)) {
      closeDialog(true);
      window.removeEventListener("keydown", onKey);
    }
  };
  window.addEventListener("keydown", onKey);

  if (btnConfirm) {
    btnConfirm.onclick = async () => {
      btnConfirm.disabled = true;
      btnConfirm.textContent = "正在准备更新...";
      if (btnCancel) {
        btnCancel.disabled = true;
        btnCancel.classList.add("hidden");
      }
      if (btnClose) btnClose.disabled = true;
      if (progressBox) progressBox.classList.remove("hidden");
      if (statusText) statusText.textContent = "正在连接并下载更新包...";

      try {
        if (!window.go || !window.go.main || !window.go.main.App || !window.go.main.App.ApplyAppUpdate) {
          throw new Error("更新接口未就绪，请重启软件后重试");
        }
        await window.go.main.App.ApplyAppUpdate();
        if (statusText) statusText.textContent = "更新包已就绪，即将自动重启应用覆盖旧版...";
        if (percentText) percentText.textContent = "100%";
        if (progressBar) progressBar.style.width = "100%";
      } catch (err) {
        if (statusText) statusText.textContent = "更新失败: " + (err && err.message ? err.message : err);
        if (btnCancel) {
          btnCancel.disabled = false;
          btnCancel.classList.remove("hidden");
          btnCancel.textContent = "关闭";
        }
        if (btnConfirm) {
          btnConfirm.disabled = false;
          btnConfirm.textContent = "重试";
        }
        if (btnClose) btnClose.disabled = false;
        P("更新失败: " + (err && err.message ? err.message : err));
      }
    };
  }
}

async function checkAndPromptAppUpdate(manual = false) {
  if (manual) {
    P("正在检测客户端软件新版本...");
  }
  try {
    if (!window.go || !window.go.main || !window.go.main.App || !window.go.main.App.CheckAppUpdate) {
      if (manual) P("软件更新接口未就绪");
      return;
    }
    const info = await window.go.main.App.CheckAppUpdate(manual);
    if (!info) {
      if (manual) P("未获取到更新信息");
      return;
    }
    if (info.available) {
      showAppUpdateDialog(info);
    } else {
      if (manual) {
        const cur = info.currentVersion || (s.lastState && s.lastState.appVersion) || "";
        P(`当前软件已是最新版本 (v${cur.replace(/^v/, "")})`);
      }
    }
  } catch (err) {
    if (manual) {
      P("检测软件更新失败: " + (err && err.message ? err.message : err));
    }
  }
}

function initAppUpdateUI() {
  if (!Fe()) return;

  Te("app-update-available", (info) => {
    if (!appUpdateDismissedThisSession && info && info.available) {
      showAppUpdateDialog(info);
    }
  });

  Te("app-update-progress", (data) => {
    const progressBox = c("appUpdateProgressBox");
    const progressBar = c("appUpdateProgressBar");
    const statusText = c("appUpdateStatusText");
    const percentText = c("appUpdatePercentText");
    if (!progressBox) return;

    progressBox.classList.remove("hidden");
    const percent = Math.min(100, Math.max(0, Math.round((data && data.percent) || 0)));
    if (progressBar) progressBar.style.width = percent + "%";
    if (percentText) percentText.textContent = percent + "%";

    if (data && data.phase === "downloading") {
      const spd = data.speedString ? ` (${data.speedString})` : "";
      if (statusText) statusText.textContent = `正在下载更新包 ${percent}%${spd}`;
    } else if (data && data.phase === "extracting") {
      if (statusText) statusText.textContent = data.message || "下载完成，正在解压校验更新包...";
    } else if (data && data.phase === "applying") {
      if (statusText) statusText.textContent = data.message || "准备完成，即将自动重启覆盖旧版本...";
    } else if (data && data.message) {
      if (statusText) statusText.textContent = data.message;
    }
  });

  // 每次启动软件后自动检测更新（每次开启软件都会提示；若本次会话点击暂不更新则本次不再弹窗）
  setTimeout(() => {
    if (!appUpdateDismissedThisSession) {
      if (s.lastState && s.lastState.appUpdateAvailable) {
        showAppUpdateDialog({
          available: true,
          currentVersion: s.lastState.appVersion,
          latestVersion: s.lastState.appLatestVersion,
          releaseNotes: s.lastState.appReleaseNotes
        });
      } else {
        checkAndPromptAppUpdate(false);
      }
    }
  }, 3500);
}

Kr();initTheme();se();_e();jr();Ft();initAppUpdateUI();setInterval(scheduleStatePoll,30000);document.addEventListener("visibilitychange",()=>{document.hidden?(s.uiPaused=!0):(s.uiPaused=!1,handleSystemResume())});});export default ia();












































