#!/usr/bin/env node
'use strict';

// Dependency-free behavioral test for the exact inline file-picker JavaScript.
// All files are synthetic; no browser file chooser, server or user data.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const html = fs.readFileSync(path.resolve(__dirname, '../r11_original.html'), 'utf8');
const scripts = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script\s*>/gi)];
assert.equal(scripts.length, 2, 'expected original R11 JS and separate Russian file picker');

class FakeElement {
  constructor(tagName, id = '') {
    this.nodeType = 1;
    this.tagName = tagName;
    this.id = id;
    this.type = tagName === 'input' ? 'file' : '';
    this.dataset = {};
    this.children = [];
    this.attributes = {};
    this.listeners = new Map();
    this.files = [];
    this.classList = {add: name => {this.className = (this.className || '') + ' ' + name;}};
    this.clicked = 0;
  }
  appendChild(child) { child.parent = this; this.children.push(child); return child; }
  matches(sel) { return sel === 'input[type="file"]' && this.tagName === 'input' && this.type === 'file'; }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  querySelectorAll(sel) {
    const found = [];
    const walk = element => {
      for (const child of element.children) {
        if (child.matches(sel)) found.push(child);
        walk(child);
      }
    };
    walk(this);
    return found;
  }
  addEventListener(type, fn) {
    const listeners = this.listeners.get(type) || [];
    listeners.push(fn);
    this.listeners.set(type, listeners);
  }
  dispatchEvent(event) { for (const fn of this.listeners.get(event.type) || []) fn(event); }
  setAttribute(name, value) {this.attributes[name] = value;}
  insertAdjacentElement(where, element) {
    assert.equal(where, 'afterend');
    const siblings = this.parent.children;
    siblings.splice(siblings.indexOf(this) + 1, 0, element);
    element.parent = this.parent;
  }
  click() {this.clicked++;}
}

const body = new FakeElement('body');
const form = body.appendChild(new FakeElement('form'));
const input = form.appendChild(new FakeElement('input', 'empxlsx'));
input.accept = '.xlsx,.csv';
let originalChange = 0;
input.addEventListener('change', () => originalChange++);

let observer;
const document = {
  nodeType: 9, body,
  querySelectorAll: selector => body.querySelectorAll(selector),
  createElement: name => new FakeElement(name),
  listeners: {},
  addEventListener(type, fn) {this.listeners[type] = fn;}
};
const context = vm.createContext({
  document,
  window: {setTimeout: fn => {fn();}},
  Event: class Event {constructor(type) {this.type = type;}},
  MutationObserver: class MutationObserver {
    constructor(callback) {this.callback = callback; observer = this;}
    observe(target, options) {
      assert.equal(target, body);
      assert.equal(options.subtree, true);
    }
  }
});
vm.runInContext(scripts[1][1], context, {filename: 'r11-russian-file-picker.js', timeout: 5000});

assert.equal(form.children.length, 3);
const button = form.children[1], status = form.children[2];
assert.equal(button.textContent, 'Выбрать файл');
assert.equal(button.type, 'button');
assert.equal(status.textContent, 'Файл не выбран');
assert.equal(status.attributes['aria-live'], 'polite');
assert.equal(input.accept, '.xlsx,.csv');
assert.equal(input.id, 'empxlsx');
button.dispatchEvent({type:'click'});
assert.equal(input.clicked, 1);
input.files = [{name:'Тестовый & <файл>.xlsx'}];
input.dispatchEvent({type:'change'});
assert.equal(status.textContent, 'Выбран файл: Тестовый & <файл>.xlsx');
assert.equal(originalChange, 1, 'original import listeners must keep working');

// R11 re-renders screens: the observer must enhance newly created inputs once.
const second = form.appendChild(new FakeElement('input', 'pointxlsx'));
second.accept = '.xlsx,.csv';
observer.callback([{addedNodes:[second]}]);
assert.equal(second.dataset.r11PickerReady, '1');
assert.equal(form.children.length, 6);
const secondButton = form.children[4], secondStatus = form.children[5];
assert.equal(secondButton.textContent, 'Выбрать файл');
assert.equal(secondStatus.textContent, 'Файл не выбран');
observer.callback([{addedNodes:[secondButton,secondStatus]}]);
assert.equal(form.children.length, 6, 'enhancement must be idempotent');
second.files = [{name:'Города.xlsx'}];
second.dispatchEvent({type:'change'});
assert.equal(secondStatus.textContent, 'Выбран файл: Города.xlsx');

// Reset clears the visible status without changing data import semantics.
input.files = [];
document.listeners.reset({target:form});
assert.equal(status.textContent, 'Файл не выбран');
assert.equal(originalChange, 2);
assert.equal(secondStatus.textContent, 'Выбран файл: Города.xlsx');
console.log('R11 Russian file picker: initial, chosen, dynamic DOM, reset and original listeners PASS');
