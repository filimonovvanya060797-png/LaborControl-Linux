// Deliberately independent of external plugins: only ESLint core checks.
// Built-ins (Array, Math, Date, etc.) are provided by ecmaVersion.
const browserGlobals = [
  'window','document','console','fetch','alert','confirm','prompt',
  'setTimeout','clearTimeout','setInterval','clearInterval',
  'requestAnimationFrame','cancelAnimationFrame',
  'location','navigator','history','performance','localStorage','sessionStorage',
  'URL','URLSearchParams','Blob','File','FileReader','FileList',
  'TextEncoder','TextDecoder','FormData','Response','Headers','Request',
  'Event','CustomEvent','MouseEvent','event','Node','Element','HTMLElement',
  'Image','DOMParser','MutationObserver','IntersectionObserver','ResizeObserver',
  'crypto','atob','btoa','open','self','structuredClone','XMLHttpRequest',
  'DecompressionStream','ReadableStream','AbortController','EventTarget',
  'CSS','getComputedStyle','Blob','FileReaderSync','WebSocket',
  'queueMicrotask','MutationObserver','MediaQueryList','Worker'
];
export default [{
  files: ['**/*.js'],
  languageOptions: {
    ecmaVersion: 'latest',
    sourceType: 'script',
    globals: Object.fromEntries(browserGlobals.map(name => [name, 'readonly']))
  },
  rules: {
    'no-undef': 'error',
    'no-dupe-args': 'error',
    // The legacy R11 bundle intentionally patches named functions in-place.
    // Auditing that pattern requires a later architectural refactor.
    'no-func-assign': 'off',
    'no-unreachable': 'error',
    'no-unsafe-finally': 'error',
    'valid-typeof': 'error'
  }
}];
