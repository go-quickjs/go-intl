// Writes testdata/collator_node.txt.gz: the order Node's Intl.Collator puts a
// word list in, for every collation locale ICU has, under every option and
// every collation type the locale supports.
//
//	node testdata/collator_node.js
//
// Run it with a Node built on the ICU that SOURCES.md anchors to; the file
// records which one it was, and the test refuses a file from another.
//
// The golden corpus covers five option sets over a few dozen locales. This
// covers every combination, over a word list chosen to reach the parts of the
// algorithm the corpus does not: contractions and their discontiguous forms,
// prefixes, expansions, script reordering, numeric runs, Hangul, kana, the Han
// orders, and text that is not in canonical order.
//
// The output is compact because the orders repeat: most locales sort most
// words as the root does. Each distinct order is written once and the cases
// refer to it.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const here = __dirname;

// The words: every string the golden corpus sorts or compares, and more.
const golden = fs.readFileSync(path.join(here, "intl_golden.txt"), "utf8")
  .split("\n").filter(l => l.includes("Intl.Collator"));
const set = new Set();
for (const line of golden) {
  for (const m of line.matchAll(/"((?:[^"\\]|\\.)*)"/g)) {
    try { set.add(JSON.parse('"' + m[1] + '"')); } catch {}
  }
}
const extra = [
  // Latin, with what European tailorings do to it.
  "a","A","á","à","â","ä","ã","å","ā","ą","æ","Æ","b","c","C","ç","č","ć","ch","Ch","CH","cz","d","ď","đ","dz","dž","e","é","è","ê","ë","ě","ę","ė","f","g","ğ","h","i","I","í","ì","î","ï","ı","İ","į","j","k","l","ł","ľ","ll","m","n","ñ","ń","ň","ng","o","ó","ò","ô","ö","õ","ø","ő","œ","p","q","r","ř","s","ś","š","ş","ß","ss","t","ť","th","þ","u","ú","ù","û","ü","ů","ű","v","w","x","y","ý","ÿ","z","ź","ż","ž","ð","ʒ",
  // Combining marks out of canonical order, discontiguous contractions, and
  // Lithuanian's dot above.
  "a\u0328\u0301","a\u0301\u0328","\u0105\u0301","a\u0323\u030a","\u1ea1\u030a","o\u031b\u0323","\u01a1\u0323","\u1ee3","i\u0307\u0301","i\u0307\u0300","i\u0307\u0303","i\u0307","\u0308","\u0301a","a\u0308\u0301",
  "ae","AE","oe","ue","aa","Aa","AA","ij","IJ","ŉ","ǆ","ǅ","Ǆ","ﬁ","fi","ffi",
  "а","б","в","г","ґ","д","е","ё","є","ж","з","и","і","ї","й","к","л","љ","м","н","њ","о","п","р","с","т","ћ","у","ў","ф","х","ц","ч","џ","ш","щ","ъ","ы","ь","э","ю","я","Я","ѐ","ѝ",
  "α","ά","β","γ","δ","ε","ζ","η","θ","ι","ϊ","ΐ","κ","λ","μ","ν","ξ","ο","π","ρ","σ","ς","τ","υ","φ","χ","ψ","ω","ώ","Ω",
  "ا","أ","إ","آ","ب","ت","ث","ج","ح","خ","د","ذ","ر","ز","س","ش","ص","ض","ط","ظ","ع","غ","ف","ق","ك","ل","لا","م","ن","ه","ة","و","ي","ى","ئ","ؤ","ء","پ","چ","ژ","گ","ک","ی","١","٢","۱",
  "א","ב","ג","ד","ה","ו","ז","ח","ט","י","כ","ך","ל","מ","ם","נ","ן","ס","ע","פ","ף","צ","ץ","ק","ר","ש","ת","שׁ",
  "क","ख","ग","घ","च","ज","ट","ड","त","द","न","प","ब","म","य","र","ल","व","श","स","ह","क्ष","ज्ञ","कि","की","कु","के","ँ","ं","ः","क़","ड़","ॐ","१","२",
  // Thai, whose vowels written first sort after the consonant.
  "ก","ข","ค","ง","จ","ฉ","ช","ด","ต","ท","น","บ","ป","พ","ม","ย","ร","ล","ว","ส","ห","อ","ฮ","เก","แก","โก","ไก","ใก","กา","กิ","กี","กึ","กุ","กู","ก่","ก้","ก๊","ก๋","กะ","เกะ","ๆ","๑",
  // Hangul: syllables, conjoining jamo, compatibility jamo.
  "가","각","간","갈","감","강","개","거","고","구","그","기","나","다","라","마","바","사","아","자","차","카","타","파","하","힣","ᄀ","ᄁ","ᅡ","ᆨ","ㄱ","ㅏ","ᄀ\u1161",
  // Kana: voicing, small kana, the length mark and the iteration marks.
  "あ","ア","ぁ","ァ","か","カ","が","ガ","か\u3099","ば","ぱ","ハ","バ","パ","ｶ","ｱ","カー","かあ","カア","かー","きゃ","キャ","っ","ッ","ゝ","ゞ","ヽ","ヾ","ん","ン","ゔ","ヴ","ㇰ","㋐",
  // Han, for pinyin, stroke, zhuyin and radical-stroke order.
  "一","丁","七","万","三","上","下","不","与","丑","且","世","丘","丙","业","东","中","丰","串","临","丸","丹","为","主","丽","举","乃","久","么","义","之","乌","乎","乏","乐","乔","乘","乙","九","乞","也","习","乡","书","买","乱","乳","乾","了","予","争","事","二","于","亏","云","互","五","井","亚","些","亡","交","亦","产","亨","享","京","亭","亮","人","亿","什","仁","仅","仆","仇","今","介","仍","从","仓","仔","他","仗","付","仙","代","令","以","仪","们","仰","件","价","任","份","仿","企","伊","伍","伏","伐","休","众","优","伙","会","伞","伟","传","伤","伦","伪","伯","估","伴","伸","似","但","位","低","住","佐","体","何","余","佛","作","你","佩","佳","使","例","供","依","侠","侦","侧","侨","侮","侯","侵","便","促","俄","俊","俗","保","信","修","俯","俱","俺","倍","倒","候","借","值","倾","假","偏","做","停","健","偶","偷","傅","傍","储","催","傲","傻","像","僚","僧","儿","允","元","兄","充","兆","先","光","克","免","兔","党","入","全","八","公","六","兰","共","关","兴","兵","其","具","典","养","兼","冀","内","冈","册","再","冒","冠","写","军","农","张","王","李","赵","刘","陈","杨","黄","周","吴","徐","孙","胡","朱","高","林","郭","马","罗","梁","宋","郑","謝","韓","唐","馮","董","蕭","程","曹","袁","鄧","許","沈","曾","彭","呂","蘇","盧","蔣","蔡","賈","魏","薛","葉","閻","潘","杜","戴","夏","鍾","汪","田","姜","范","方","石","姚","譚","廖","鄒","熊","金","陸","郝","孔","白","崔","康","毛","邱","秦","江","史","顧","邵","孟","龍","萬","段","漕","錢","湯","尹","黎","易","常","武","喬","賀","賴","龔","文","\u3400","\u9FA0","\u{20000}","\u{2A6D6}","〇","々","〆",
  // Digits, for numeric sorting and for other scripts' digits.
  "0","1","2","9","10","11","02","002","20","100","1000","1,000","1.5","1.05","12345678901234567890","０","１","²","½","①","Ⅳ","ⅳ","٣","۳","३","๓",
  // Spaces, punctuation and symbols, the variable characters.
  " ","-","_",".",",",";",":","!","?","'","\"","(",")","[","]","{","}","@","*","/","\\","&","#","%","`","^","+","<","=",">","|","~","$","€","£","¥","¢","©","®","°","±","§","¶","·","•","…","–","—","‘","’","“","”","«","»","\u00a0","\u200b","\u200d","\u00ad",
  "a b","a-b","ab","a_b","a.b","a'b","co-op","coop","co op","e-mail","email","e mail",
  "😀","😃","😂","🙂","👍","👍🏽","❤","❤️","🇺🇸","🇫🇷","👨‍👩‍👧","☺","★","☆","♠","→","∑","∞","√",
  "ა","ბ","Ա","ա","ሀ","ለ","ក","ខ","ກ","ຂ","က","ခ","අ","ආ","ཀ","ཁ","ᏣᎳᎩ","ꭰ","ߊ","𞤀","Ꭰ","ᐊ","ᚠ","ᛗ","ⴰ","ⵣ",
  // Controls, unassigned and private-use code points, the ends of the range.
  "\u0000","\u0001","\ufffd","\uffff","\u{10000}","\u{10ffff}","\u{e0001}","\u{f0000}","\u0378","\u2fff",
  "Straße","Strasse","STRASSE","Müller","Mueller","Muller","Göbel","Goethe","Götz","Øre","Ørsted","Aarhus","Aalborg","Ångström","Ärger","Zürich",
  "España","Espana","llama","luz","chico","cuna","Ñandú","ñu","nube",
  "hàng","háng","hạng","hãng","hảng","hang","Ăn","ân","ơn","ưa","đ","Đ",
  "ẞ","ǃ","ǂ","ʔ","ʻ","ʼ",
];
for (const w of extra) set.add(w);
// A Go string cannot hold a lone surrogate, so none is used.
const words = [...set].filter(w => w.isWellFormed());

// The locales: every one ICU has a collation entry for, and a few that reach
// one through an alias.
const locales = fs.readFileSync(path.join(here, "collator_locales.txt"), "utf8")
  .split("\n").map(s => s.trim()).filter(s => s && !s.startsWith("#"));

const optionSets = [
  {}, { sensitivity: "base" }, { sensitivity: "accent" }, { sensitivity: "case" }, { sensitivity: "variant" },
  { numeric: true }, { caseFirst: "upper" }, { caseFirst: "lower" }, { caseFirst: "false" },
  { ignorePunctuation: true }, { ignorePunctuation: false },
  { usage: "search" }, { usage: "search", sensitivity: "base" },
  { sensitivity: "case", caseFirst: "upper" }, { numeric: true, ignorePunctuation: true, sensitivity: "accent" },
];
const collations = Intl.supportedValuesOf("collation");

const orders = new Map();
const cases = [];
for (const loc of locales) {
  const configs = optionSets.map(o => [loc, o]);
  for (const co of collations) {
    if (new Intl.Collator(loc, { collation: co }).resolvedOptions().collation === co) {
      configs.push([loc, { collation: co }]);
    }
    if (new Intl.Collator(loc + "-u-co-" + co).resolvedOptions().collation === co) {
      configs.push([loc + "-u-co-" + co, {}]);
    }
  }
  for (const [tag, opts] of configs) {
    const c = new Intl.Collator(tag, opts);
    const resolved = c.resolvedOptions();
    // A locale Node does not support falls back to Node's default locale,
    // which is negotiation, not collation.
    if (resolved.locale.split("-u-")[0].toLowerCase() !== tag.split("-u-")[0].toLowerCase()) continue;
    const idx = words.map((_, i) => i);
    idx.sort((a, b) => c.compare(words[a], words[b]));
    let order = String(idx[0]);
    for (let i = 1; i < idx.length; i++) {
      order += (c.compare(words[idx[i - 1]], words[idx[i]]) === 0 ? "=" : "<") + idx[i];
    }
    if (!orders.has(order)) orders.set(order, orders.size);
    cases.push(["case", tag, JSON.stringify(opts), JSON.stringify(resolved), orders.get(order)].join("\t"));
  }
}

const lines = [
  `# node ${process.version} ICU ${process.versions.icu}`,
  "words\t" + JSON.stringify(words),
  ...[...orders].map(([order, id]) => `order\t${id}\t${order}`),
  ...cases,
];
const out = path.join(here, "collator_node.txt.gz");
fs.writeFileSync(out, zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
console.log(`${cases.length} cases, ${orders.size} distinct orders, ${words.length} words -> ${out}`);
