// Writes testdata/segmenter_node.txt.gz: how Node's Intl.Segmenter breaks
// text, in every granularity and a few locales whose rules differ.
//
//	node testdata/segmenter_node.js
//
// Each line is [locale, granularity, text, boundaries, wordLike]: the text
// as UTF-16 code units (JSON cannot carry a lone surrogate), the segments'
// start offsets and the text's length, and, for words, 1 where a segment
// is word-like.
//
// ICU keeps its dictionary break engines for the whole process once made,
// and an engine claims every character of its set; the recording first
// breaks text in every dictionary script so that all of them are made, the
// state go-intl reckons in.
"use strict";
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

for (const t of ["ภาษาไทย", "ພາສາລາວ", "ភាសាខ្មែរ", "မြန်မာဘာသာ", "日本語", "中文", "ひらがな", "カタカナ"]) {
  [...new Intl.Segmenter("en", { granularity: "word" }).segment(t)];
}

const sentences = [
  "Hello, world! How are you? I'm fine; thanks.",
  "Mr. Smith went to Washington. He arrived at 3 p.m. on Jan. 5th.",
  "The quick (\"brown\") fox—jumps over the lazy dog… 3.14159, 1,000,000.00 and 42%!",
  "e-mail: someone@example.com; URL: https://example.com/a/b?c=d&e=f#g",
  "can't won't y'all O'Neill rock'n'roll ’twas",
  "Κάποτε ήταν ένας βασιλιάς; ζούσε σε ένα παλάτι. Τέλος;",
  "Привет, мир! Как дела? Всё хорошо.",
  "שלום עולם! מה שלומך? צה״ל ו־ג׳ירפה.",
  "مرحبا بالعالم! كيف حالك؟ ١٢٣٤ و٥٦٧.",
  "नमस्ते दुनिया। आप कैसे हैं? क्षत्रिय श्री।",
  "হ্যালো বিশ্ব। আপনি কেমন আছেন?",
  "สวัสดีครับ ยินดีต้อนรับสู่ประเทศไทย วันนี้อากาศดีมาก ๆ ฯลฯ",
  "ມະນຸດທຸກຄົນເກີດມາມີອິດສະຫຼະ ແລະ ສະເໝີພາບກັນ",
  "មនុស្សទាំងអស់កើតមកមានសេរីភាព និងសមភាព",
  "လူတိုင်းသည် တူညီလွတ်လပ်သော ဂုဏ်သိက္ခါဖြင့် မွေးဖွားလာသူများ ဖြစ်သည်။",
  "人人生而自由，在尊严和权利上一律平等。他们赋有理性和良心。",
  "すべての人間は、生まれながらにして自由であり、かつ、尊厳と権利とについて平等である。",
  "コンピューターのソフトウェアをダウンロードしました。ｶﾀｶﾅとﾊﾝｶｸもあります。",
  "東京タワーに行きました。ラーメンを食べました！１２３ＡＢＣ。",
  "모든 인간은 태어날 때부터 자유로우며 그 존엄과 권리에 있어 동등하다.",
  "👨‍👩‍👧‍👦 family, 👍🏽 thumbs, 🇺🇸🇫🇷 flags, 🏴󠁧󠁢󠁳󠁣󠁴󠁿 Scotland, 1️⃣ keycap, ❤️ heart.",
  "éé̂ ǟ 각 ᄀ가 \r\n\r\n\n",
  "line one\nline two\r\nline three line four para",
  "\ud800 lone \udc00 surrogates \ud83d",
  "abc123def 3.5.6 1,2,3 a_b a.b a:b 'quoted' \"double\" 12:30 1/2",
  "Hello.World! Hello. World. hello? world! (Hello.) \"Hello.\" Hello...World",
  "日本語とEnglishの混在テキスト。中文English混合。",
  "ﾃｽﾄ ﾃﾞｰﾀ ｺﾝﾋﾟｭｰﾀｰ ﾊﾟｿｺﾝ",
  "ー長音から始まるテキスト、ーー。ｰｰ。",
  "Ａｌｐｈａ　ｂｅｔａ　１２３　㍿ ㌔ ㈱ ⑴ ﬁ",
];

// A seeded generator, so the recording is the same each time.
let seed = 0x2545f491;
function rand(n) {
  seed ^= seed << 13; seed >>>= 0;
  seed ^= seed >>> 17;
  seed ^= seed << 5; seed >>>= 0;
  return seed % n;
}
function range(lo, hi) {
  const out = [];
  for (let c = lo; c <= hi; c++) out.push(c);
  return out;
}
const pools = [
  [..."abcdefghijklmnopqrstuvwxyzABCXYZ"].map(c => c.codePointAt(0)),
  [..."0123456789"].map(c => c.codePointAt(0)),
  [..." .,;:!?'\"()-_/@#%&*+=[]{}…。、「」！？＇"].map(c => c.codePointAt(0)),
  [0x0d, 0x0a, 0x85, 0x2028, 0x2029, 0x09, 0x20, 0xa0, 0x3000],
  range(0x0300, 0x0306).concat([0x200c, 0x200d, 0xfe0f, 0x20e3]),
  [0x1f600, 0x1f44d, 0x1f468, 0x1f469, 0x1f467, 0x2764, 0x1f3fb, 0x1f3fd, 0x1f1fa, 0x1f1f8, 0x1f1eb,
    0x1f1f7, 0x1f3f4, 0xe0067, 0xe0062, 0xe007f, 0x0031],
  range(0x1100, 0x1104).concat(range(0x1161, 0x1164), range(0x11a8, 0x11ab), [0xac00, 0xac01, 0xd7a3]),
  range(0x0915, 0x091a).concat([0x093f, 0x094d, 0x0930, 0x0937, 0x0924, 0x0964, 0x0995, 0x09cd, 0x09b7]),
  range(0x0e01, 0x0e3a).concat(range(0x0e40, 0x0e4e)),
  range(0x0e81, 0x0e8a).concat(range(0x0eb0, 0x0ebc), range(0x0ec0, 0x0ec6), [0x0edc]),
  range(0x1780, 0x17b3).concat(range(0x17b6, 0x17d3)),
  range(0x1000, 0x102a).concat(range(0x102b, 0x103e)),
  [..."人生而自由在尊严和权利上一律平等日本語東京中文漢字"].map(c => c.codePointAt(0)),
  range(0x3041, 0x3096).concat([0x309d, 0x309e]),
  range(0x30a1, 0x30fa).concat([0x30fc, 0x30fb, 0xff70, 0xff9e, 0xff9f], range(0xff66, 0xff6f)),
  range(0xff10, 0xff19).concat(range(0xff21, 0xff26), [0x3371, 0x337f, 0x3231, 0xfb01, 0x2474]),
  [..."שלוםעבריתמרחבا"].map(c => c.codePointAt(0)).concat([0x05f3, 0x05f4, 0x05be, 0x0627, 0x0644, 0x0661]),
  [0xd800, 0xdbff, 0xdc00, 0xdfff],
];
const random = [];
for (let i = 0; i < 3000; i++) {
  const n = 1 + rand(30);
  // Mostly one or two pools, so that runs of one script form.
  const a = pools[rand(pools.length)], b = pools[rand(pools.length)];
  let s = "";
  for (let j = 0; j < n; j++) {
    const p = rand(4) === 0 ? b : a;
    s += String.fromCodePoint(p[rand(p.length)]);
  }
  random.push(s);
}

const units = s => { const u = []; for (let i = 0; i < s.length; i++) u.push(s.charCodeAt(i)); return u; };
const lines = [`# Node ${process.version}, ICU ${process.versions.icu}`];
const cases = [
  ["en", sentences.concat(random)],
  ["el", sentences],
  ["en-US-u-va-posix", sentences],
  ["ja", sentences],
  ["th", sentences],
  ["zh", sentences],
  ["de", sentences.slice(0, 5)],
];
for (const [locale, texts] of cases) {
  for (const granularity of ["grapheme", "word", "sentence"]) {
    const seg = new Intl.Segmenter(locale, { granularity });
    for (const text of texts) {
      const bounds = [], wordLike = [];
      for (const s of seg.segment(text)) {
        bounds.push(s.index);
        if (granularity === "word") wordLike.push(s.isWordLike ? 1 : 0);
      }
      bounds.push(text.length);
      lines.push(JSON.stringify([locale, granularity, units(text), bounds, wordLike]));
    }
  }
}
fs.writeFileSync(path.join(__dirname, "segmenter_node.txt.gz"), zlib.gzipSync(lines.join("\n") + "\n", { level: 9 }));
