export type NginxRewriteTemplate = { id: string; body: string };

function n(s: string) {
  return s
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .join("\n");
}

export const NGINX_REWRITE_TEMPLATES: NginxRewriteTemplate[] = [
  {
    id: "discuzx",
    body: `rewrite ^([^\\.]*)/topic-(.+)\\.html$ $1/portal.php?mod=topic&topic=$2 last;
rewrite ^([^\\.]*)/article-([0-9]+)-([0-9]+)\\.html$ $1/portal.php?mod=view&aid=$2&page=$3 last;
rewrite ^([^\\.]*)/forum-(\\w+)-([0-9]+)\\.html$ $1/forum.php?mod=forumdisplay&fid=$2&page=$3 last;
rewrite ^([^\\.]*)/thread-([0-9]+)-([0-9]+)-([0-9]+)\\.html$ $1/forum.php?mod=viewthread&tid=$2&extra=page%3D$4&page=$3 last;
rewrite ^([^\\.]*)/group-([0-9]+)-([0-9]+)\\.html$ $1/forum.php?mod=group&fid=$2&page=$3 last;
rewrite ^([^\\.]*)/space-(username|uid)-(.+)\\.html$ $1/home.php?mod=space&$2=$3 last;
rewrite ^([^\\.]*)/blog-([0-9]+)-([0-9]+)\\.html$ $1/home.php?mod=space&uid=$2&do=blog&id=$3 last;
rewrite ^([^\\.]*)/(fid|tid)-([0-9]+)\\.html$ $1/index.php?action=$2&value=$3 last;
rewrite ^([^\\.]*)/([a-z]+[a-z0-9_]*)-([a-z0-9_\\-]+)\\.html$ $1/plugin.php?id=$2:$3 last;
if (!-e $request_filename) {
    return 404;
}`,
  },
  {
    id: "drupal",
    body: `if (!-e $request_filename) {
    rewrite ^/(.*)$ /index.php?q=$1 last;
}`,
  },
  {
    id: "ecshop",
    body: `if (!-e $request_filename) {
    rewrite "^/index\\.html" /index.php last;
    rewrite "^/category$" /index.php last;
    rewrite "^/feed-c([0-9]+)\\.xml$" /feed.php?cat=$1 last;
    rewrite "^/feed-b([0-9]+)\\.xml$" /feed.php?brand=$1 last;
    rewrite "^/feed\\.xml$" /feed.php last;
    rewrite "^/category-([0-9]+)-b([0-9]+)-min([0-9]+)-max([0-9]+)-attr([^-]*)-([0-9]+)-(.+)-([a-zA-Z]+)(.*)\\.html$" /category.php?id=$1&brand=$2&price_min=$3&price_max=$4&filter_attr=$5&page=$6&sort=$7&order=$8 last;
    rewrite "^/category-([0-9]+)-b([0-9]+)-min([0-9]+)-max([0-9]+)-attr([^-]*)(.*)\\.html$" /category.php?id=$1&brand=$2&price_min=$3&price_max=$4&filter_attr=$5 last;
    rewrite "^/category-([0-9]+)-b([0-9]+)-([0-9]+)-(.+)-([a-zA-Z]+)(.*)\\.html$" /category.php?id=$1&brand=$2&page=$3&sort=$4&order=$5 last;
    rewrite "^/category-([0-9]+)-b([0-9]+)-([0-9]+)(.*)\\.html$" /category.php?id=$1&brand=$2&page=$3 last;
    rewrite "^/category-([0-9]+)-b([0-9]+)(.*)\\.html$" /category.php?id=$1&brand=$2 last;
    rewrite "^/category-([0-9]+)(.*)\\.html$" /category.php?id=$1 last;
    rewrite "^/goods-([0-9]+)(.*)\\.html" /goods.php?id=$1 last;
    rewrite "^/article_cat-([0-9]+)-([0-9]+)-(.+)-([a-zA-Z]+)(.*)\\.html$" /article_cat.php?id=$1&page=$2&sort=$3&order=$4 last;
    rewrite "^/article_cat-([0-9]+)-([0-9]+)(.*)\\.html$" /article_cat.php?id=$1&page=$2 last;
    rewrite "^/article_cat-([0-9]+)(.*)\\.html$" /article_cat.php?id=$1 last;
    rewrite "^/article-([0-9]+)(.*)\\.html$" /article.php?id=$1 last;
    rewrite "^/brand-([0-9]+)-c([0-9]+)-([0-9]+)-(.+)-([a-zA-Z]+)\\.html" /brand.php?id=$1&cat=$2&page=$3&sort=$4&order=$5 last;
    rewrite "^/brand-([0-9]+)-c([0-9]+)-([0-9]+)(.*)\\.html" /brand.php?id=$1&cat=$2&page=$3 last;
    rewrite "^/brand-([0-9]+)-c([0-9]+)(.*)\\.html" /brand.php?id=$1&cat=$2 last;
    rewrite "^/brand-([0-9]+)(.*)\\.html" /brand.php?id=$1 last;
    rewrite "^/tag-(.*)\\.html" /search.php?keywords=$1 last;
    rewrite "^/snatch-([0-9]+)\\.html$" /snatch.php?id=$1 last;
    rewrite "^/group_buy-([0-9]+)\\.html$" /group_buy.php?act=view&id=$1 last;
    rewrite "^/auction-([0-9]+)\\.html$" /auction.php?act=view&id=$1 last;
    rewrite "^/exchange-id([0-9]+)(.*)\\.html$" /exchange.php?id=$1&act=view last;
    rewrite "^/exchange-([0-9]+)-min([0-9]+)-max([0-9]+)-([0-9]+)-(.+)-([a-zA-Z]+)(.*)\\.html$" /exchange.php?cat_id=$1&integral_min=$2&integral_max=$3&page=$4&sort=$5&order=$6 last;
    rewrite "^/exchange-([0-9]+)-([0-9]+)-(.+)-([a-zA-Z]+)(.*)\\.html$" /exchange.php?cat_id=$1&page=$2&sort=$3&order=$4 last;
    rewrite "^/exchange-([0-9]+)-([0-9]+)(.*)\\.html$" /exchange.php?cat_id=$1&page=$2 last;
    rewrite "^/exchange-([0-9]+)(.*)\\.html$" /exchange.php?cat_id=$1 last;
}`,
  },
  {
    id: "emlog",
    body: `index index.php index.html;
if (!-e $request_filename) {
    rewrite ^/(.*)$ /index.php last;
}`,
  },
  {
    id: "laravel-filamentphp",
    body: `# laravel-filamentphp
try_files $uri $uri/ /index.php$is_args$query_string;`,
  },
  {
    id: "laravel-livewire",
    body: `# laravel-livewire
try_files $uri $uri/ /index.php$is_args$query_string;`,
  },
  {
    id: "laravel",
    body: `# laravel
try_files $uri $uri/ /index.php?$query_string;`,
  },
  {
    id: "laravel5",
    body: `try_files $uri $uri/ /index.php$is_args$query_string;`,
  },
  {
    id: "maccms",
    body: `rewrite ^/vod-(.*)$ /index.php?m=vod-$1 break;
rewrite ^/art-(.*)$ /index.php?m=art-$1 break;
rewrite ^/gbook-(.*)$ /index.php?m=gbook-$1 break;
rewrite ^/label-(.*)$ /index.php?m=label-$1 break;
rewrite ^/map-(.*)$ /index.php?m=map-$1 break;`,
  },
  {
    id: "thinkphp",
    body: `if (!-e $request_filename) {
    rewrite ^(.*)$ /index.php?s=$1 last;
}`,
  },
  {
    id: "typecho",
    body: `if (!-e $request_filename) {
    rewrite ^(.*)$ /index.php$1 last;
}`,
  },
  {
    id: "wordpress",
    body: `try_files $uri $uri/ /index.php?$args;
rewrite /wp-admin$ $scheme://$host$uri/ permanent;`,
  },
  {
    id: "discuz",
    body: `rewrite ^/archiver/((fid|tid)-[\\w\\-]+\\.html)$ /archiver/index.php?$1 last;
rewrite ^/forum-([0-9]+)-([0-9]+)\\.html$ /forumdisplay.php?fid=$1&page=$2 last;
rewrite ^/thread-([0-9]+)-([0-9]+)-([0-9]+)\\.html$ /viewthread.php?tid=$1&extra=page%3D$3&page=$2 last;
rewrite ^/space-(username|uid)-(.+)\\.html$ /space.php?$1=$2 last;
rewrite ^/tag-(.+)\\.html$ /tag.php?name=$1 last;`,
  },
  {
    id: "phpwind",
    body: `rewrite ^(.*)-htm-(.*)$ $1.php?$2 last;
rewrite ^(.*)/simple/([a-z0-9_]+\\.html)$ $1/simple/index.php?$2 last;`,
  },
];

export const NGINX_REWRITE_OPTIONS = NGINX_REWRITE_TEMPLATES.map((t) => ({ value: t.id, label: t.id }));

export function matchNginxRewrite(raw: string): string | undefined {
  const cur = n(raw);
  if (!cur) return undefined;
  const hits = NGINX_REWRITE_TEMPLATES.filter((t) => n(t.body) === cur);
  return hits[0]?.id;
}

export function nginxRewriteBody(id: string | undefined): string {
  if (!id) return "";
  return NGINX_REWRITE_TEMPLATES.find((t) => t.id === id)?.body ?? "";
}
