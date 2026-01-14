package main

const (
	bypassHeadlessDetectJS = `(function(w, n, wn) {
		// Pass the Webdriver Test.
		Object.defineProperty(n, 'webdriver', {
		  get: () => false,
		});

		// Pass the Plugins Length Test.
		Object.defineProperty(n, 'plugins', {
		  get: () => [1, 2, 3, 4, 5],
		});

		// Pass the Languages Test.
		Object.defineProperty(n, 'languages', {
		  get: () => ['zh-CN', 'zh'],
		});

		// Pass the Chrome Test.
		w.chrome = {
		  runtime: {},
		};

		// Pass the Permissions Test.
		const originalQuery = wn.permissions.query;
		return wn.permissions.query = (parameters) => (
		  parameters.name === 'notifications' ?
			Promise.resolve({ state: Notification.permission }) :
			originalQuery(parameters)
		);
	})(window, navigator, window.navigator);`

	initHookJS = `
		// ==================== 常量定义 ====================
		const DOM_EVENT_FLAG = 'data-dom-events';
		const LINK_ATTRS = ['href', 'src', 'data-href', 'data-url', 'data-link'];
		const JS_PROTOCOL = 'javascript:';

		// ==================== 任务队列 ====================
		// 使用 Promise 队列确保任务顺序执行
		const TaskQueue = {
			queue: Promise.resolve(),
			pendingCount: 0,
			
			// 添加任务到队列，确保顺序执行
			add(task) {
				this.pendingCount++;
				this.queue = this.queue.then(() => this.runTask(task)).catch(() => {});
				return this.queue;
			},
			
			// 执行单个任务
			async runTask(task) {
				window.__flamingoStability.recordTask();
				try {
					await task();
				} catch (e) {}
				// 等待 DOM 更新完成
				await this.waitForDOMUpdate();
				this.pendingCount--;
			},
			
			// 等待 DOM 更新完成（使用 requestAnimationFrame + 微任务）
			waitForDOMUpdate() {
				return new Promise(resolve => {
					requestAnimationFrame(() => {
						// 微任务确保在渲染后执行
						queueMicrotask(resolve);
					});
				});
			},
			
			// 批量添加任务
			addAll(tasks) {
				tasks.forEach(task => this.add(task));
				return this.queue;
			},
			
			// 检查队列是否空闲
			isIdle() {
				return this.pendingCount === 0;
			}
		};
		
		// ==================== 稳定性监测 ====================
		// 全局稳定性状态
		window.__flamingoStability = {
			lastMutationTime: Date.now(),
			lastTaskTime: Date.now(),
			
			// 更新 mutation 时间
			recordMutation() {
				this.lastMutationTime = Date.now();
			},
			
			// 更新任务时间
			recordTask() {
				this.lastTaskTime = Date.now();
			},
			
			// 检查是否稳定（无 mutation 且任务队列空闲）
			isStable(quietPeriodMs) {
				const now = Date.now();
				const mutationQuiet = (now - this.lastMutationTime) >= quietPeriodMs;
				const taskQuiet = (now - this.lastTaskTime) >= quietPeriodMs;
				const queueIdle = TaskQueue.isIdle();
				return mutationQuiet && taskQuiet && queueIdle;
			}
		};

		// ==================== 工具函数 ====================
		// 随机选择数组中的一项
		function pickone(arr) {
			return arr[Math.floor(Math.random() * arr.length)];
		}

		// 从指定字符区间生成指定长度的字符串
		function random(chars, len) {
			let result = '';
			for (let i = 0; i < len; i++) {
				result += chars.charAt(Math.floor(Math.random() * chars.length));
			}
			return result;
		}

		// 安全发送链接
		function safeSendLink(url, source) {
			try {
				const absUrl = new URL(url, document.baseURI).href;
				window.sendLink(JSON.stringify({url: absUrl, source: source}));
			} catch (e) {}
		}

		// 判断是否为 JS 伪协议
		function isJsProtocol(link) {
			return link && link.toLowerCase().startsWith(JS_PROTOCOL);
		}

		// 安全执行 JS 代码
		function safeEval(code) {
			try { eval(code); } catch (e) {}
		}

		// 处理链接：执行 JS 伪协议或发送链接
		function processLink(link, source) {
			if (!link) return;
			if (isJsProtocol(link)) {
				TaskQueue.add(() => safeEval(link));
			} else {
				safeSendLink(link, source);
			}
		}

		// ==================== Hook 函数 ====================
		// hook 打开、关闭标签页函数
		window.open = function (url) {
			safeSendLink(url, 'open');
		};
		window.close = function () {};

		// 锁定 open/close
		['open', 'close'].forEach((action) => {
			Object.defineProperty(window, action, {
				writable: false, 
				configurable: false,
			});
		});

		// 劫持表单重置，避免清空表单内容
		HTMLFormElement.prototype.reset = function() {};
		Object.defineProperty(HTMLFormElement.prototype, 'reset', {
			writable: false, 
			configurable: false,
		});

		// ==================== 事件记录 ====================
		// 记录元素绑定的事件
		function recordEvent(element, eventName) {
			const existing = element.getAttribute(DOM_EVENT_FLAG);
			if (existing) {
				element.setAttribute(DOM_EVENT_FLAG, existing + ',' + eventName);
			} else {
				element.setAttribute(DOM_EVENT_FLAG, eventName);
			}
		}

		// 记录 DOM0 事件注册（使用 onXYZ 属性绑定方式）
		const DOM_EVENTS = ['abort', 'afterprint', 'animationend', 'animationiteration', 'animationstart', 'beforeprint', 'beforeunload', 'blur', 'canplay', 'canplaythrough', 'change', 'click', 'contextmenu', 'copy', 'cut', 'dblclick', 'drag', 'dragend', 'dragenter', 'dragleave', 'dragover', 'dragstart', 'drop', 'durationchange', 'ended', 'error', 'focus', 'focusin', 'focusout', 'fullscreenchange', 'fullscreenerror', 'hashchange', 'input', 'invalid', 'keydown', 'keypress', 'keyup', 'load', 'loadeddata', 'loadedmetadata', 'loadstart', 'message', 'mousedown', 'mouseenter', 'mouseleave', 'mousemove', 'mouseout', 'mouseover', 'mouseup', 'mousewheel', 'offline', 'online', 'open', 'pagehide', 'pageshow', 'paste', 'pause', 'play', 'playing', 'popstate', 'progress', 'ratechange', 'reset', 'resize', 'scroll', 'search', 'seeked', 'seeking', 'select', 'show', 'stalled', 'storage', 'submit', 'suspend', 'timeupdate', 'toggle', 'touchcancel', 'touchend', 'touchmove', 'touchstart', 'transitionend', 'unload', 'volumechange', 'waiting', 'wheel'];

		// 使用 WeakMap 存储原始事件处理器
		const eventHandlers = new WeakMap();

		DOM_EVENTS.forEach((eName) => {
			const propName = 'on' + eName;
			const originalDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, propName);
			
			Object.defineProperty(HTMLElement.prototype, propName, {
				configurable: false,
				enumerable: true,
				get: function() {
					const handlers = eventHandlers.get(this);
					return handlers ? handlers[eName] : null;
				},
				set: function(handler) {
					// 记录事件绑定
					recordEvent(this, eName);
					// 存储处理器到元素
					let handlers = eventHandlers.get(this);
					if (!handlers) {
						handlers = {};
						eventHandlers.set(this, handlers);
					}
					handlers[eName] = handler;
					// 使用原始 setter（如果存在）
					if (originalDescriptor && originalDescriptor.set) {
						originalDescriptor.set.call(this, handler);
					}
				}
			});
		});

		// 记录 DOM2 事件注册（使用 addEventListener 方式）
		const _addEventListener = Element.prototype.addEventListener;
		Element.prototype.addEventListener = function(type, listener, options) {
			recordEvent(this, type);
			_addEventListener.call(this, type, listener, options);
		};
	`

	mutationObserverJS = `(function(){
		// 处理新增节点中的链接
		function processAddedNode(node) {
			if (node.nodeType !== Node.ELEMENT_NODE) return;
			
			node.querySelectorAll('a').forEach((aNode) => {
				let link = null;
				LINK_ATTRS.some(attr => {
					const val = aNode.getAttribute(attr);
					if (val) { link = val; return true; }
					return false;
				});
				processLink(link, 'dom');
			});
		}

		// 处理属性变化
		function processAttributeChange(mutation) {
			if (mutation.target.nodeType !== Node.ELEMENT_NODE) return;
			if (!LINK_ATTRS.includes(mutation.attributeName)) return;
			
			const link = mutation.target.getAttribute(mutation.attributeName);
			processLink(link, 'dom');
		}

		// 创建观察器实例
		const observer = new MutationObserver((mutations) => {
			// 记录 mutation 时间
			window.__flamingoStability.recordMutation();
			
			mutations.forEach((mutation) => {
				if (mutation.type === 'childList') {
					mutation.addedNodes.forEach(processAddedNode);
				} else if (mutation.type === 'attributes') {
					processAttributeChange(mutation);
				}
			});
		});
	
		// 启动观察
		observer.observe(document.documentElement, {
			subtree: true,
			childList: true,
			attributes: true,
			attributeFilter: LINK_ATTRS
		});
	})();`

	// 填充和提交表单
	fillAndSubmitFormsJS = `(function(){
		const corpus = {
			digit: '123456789',
			letter: 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ',
			symbol: '~!@#$%^&*()',
			year: ['1990', '1991', '1992', '1993', '1994', '1995'],
			month: ['01', '02', '03', '04', '05', '06', '07', '08', '09', '10', '11', '12'],
			day: ['01', '07', '10', '23'],
			surname: ['张', '李', '王'],
			name: ['伟', '芳', '娜', '敏', '静'],
			lastname: ['zhang', 'li', 'wang'],
			firstname: ['wei', 'fang', 'na', 'min', 'jing'],
			address: ['北京市朝阳区', '上海市闵行区', '广东省广州市', '广东省深圳市'],
			domain: ['.com', '.net', '.tech'],
		};

		// 生成各类型数据
		const generators = {
			phone: () => '139' + random(corpus.digit, 8),
			email: () => pickone(corpus.firstname) + '.' + pickone(corpus.lastname) + '@' + random(corpus.digit, 5) + pickone(corpus.domain),
			url: () => 'https://www.' + random(corpus.digit, 5) + pickone(corpus.domain),
			date: () => pickone(corpus.year) + pickone(corpus.month) + pickone(corpus.day),
			address: () => pickone(corpus.address),
			password: () => random(corpus.letter, 4) + random(corpus.symbol, 2) + random(corpus.digit, 4),
			text: () => 'flamingo',
			textarea: () => 'tested by flamingo',
		};

		// 根据 name 属性推断文本输入类型
		function inferTextType(name) {
			if (/((number)|(phone))|(^tel)/i.test(name)) return 'phone';
			if (/mail|email/i.test(name)) return 'email';
			if (/url|website|blog|homepage/i.test(name)) return 'url';
			if (/(date)|(^birth)/i.test(name)) return 'date';
			if (/^addr/i.test(name)) return 'address';
			return 'text';
		}

		// 生成日期时间相关值
		function generateDateTimeValue(type) {
			const year = pickone(corpus.year);
			const month = pickone(corpus.month);
			const day = pickone(corpus.day);
			
			const formats = {
				'month': () => year + '-' + month,
				'week': () => year + '-W10',
				'date': () => year + '-' + month + '-' + day,
				'datetime-local': () => year + '-' + month + '-' + day + 'T10:00',
				'time': () => '10:00',
			};
			return formats[type]();
		}

		// 生成数值类型值
		function generateNumberValue(ele) {
			const min = ele.hasAttribute('min') ? parseInt(ele.min) : null;
			const max = ele.hasAttribute('max') ? parseInt(ele.max) : null;
			
			if (min !== null && max !== null) {
				return Math.floor(Math.random() * (max - min + 1) + min);
			} else if (min !== null) {
				return min + 1;
			} else if (max !== null) {
				return max - 1;
			}
			return random(corpus.digit, 1);
		}

		// 填充单个输入元素
		function fillInput(ele) {
			const type = ele.type;
			
			switch (type) {
				case 'text':
				case 'search':
					ele.value = generators[inferTextType(ele.name)]();
					break;
				case 'password':
					ele.value = generators.password();
					break;
				case 'radio':
				case 'checkbox':
					ele.checked = true;
					break;
				case 'month':
				case 'week':
				case 'date':
				case 'datetime-local':
				case 'time':
					ele.value = generateDateTimeValue(type);
					break;
				case 'email':
					ele.value = generators.email();
					break;
				case 'number':
				case 'range':
					ele.value = generateNumberValue(ele);
					break;
				case 'url':
					ele.value = generators.url();
					break;
			}
		}

		// 创建隐藏的表单提交 target
		const IFRAME_NAME = 'flamingo_hidden_iframe';
		const iframe = document.createElement('iframe');
		iframe.style.display = 'none';
		iframe.name = IFRAME_NAME;
		document.body.appendChild(iframe);
		
		// 填充所有表单
		const forms = Array.from(document.forms);
		forms.forEach((form) => {
			form.setAttribute('target', IFRAME_NAME);
			
			Array.from(form.elements).forEach((ele) => {
				if (ele.nodeName === 'INPUT') {
					fillInput(ele);
				} else if (ele.nodeName === 'TEXTAREA') {
					ele.value = generators.textarea();
				}
			});
		});

		// 提交单个表单
		function submitForm(form) {
			try {
				form.submit();
			} catch (e) {
				// 处理表单元素的 id 或 name 属性值为 submit 的情况
				const buttons = Array.from(form.elements).filter(el => 
					(el.nodeName === 'INPUT' && (el.type === 'submit' || el.type === 'button')) || 
					el.nodeName === 'BUTTON'
				);
				// 将按钮点击加入队列
				buttons.forEach((btn) => {
					TaskQueue.add(() => {
						try { btn.click(); } catch (e) {}
					});
				});
			}
		}

		// 将所有表单提交加入队列
		forms.forEach((form) => {
			TaskQueue.add(() => submitForm(form));
		});
	})();`

	// 遍历元素，收集链接
	collectLinksJS = `(function(){
		const treeWalker = document.createTreeWalker(
			document.documentElement,
			NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_COMMENT,
			{ acceptNode: () => NodeFilter.FILTER_ACCEPT }
		);

		// 检测注释里的完整 URL
		const urlRe = /https?:\/\/(?:www\.)?[-a-zA-Z0-9@:%._+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b(?:[-a-zA-Z0-9()@:%_+.~#?&/=]*)/gi;
		
		while (treeWalker.nextNode()) {
			const node = treeWalker.currentNode;
			
			if (node.nodeType === Node.COMMENT_NODE) {
				// 注释节点：提取所有 URL
				const content = node.nodeValue;
				let match;
				while ((match = urlRe.exec(content)) !== null) {
					safeSendLink(match[0], 'comment');
				}
				urlRe.lastIndex = 0; // 重置正则状态
			} else {
				// 元素节点：检查链接属性
				LINK_ATTRS.forEach((attrName) => {
					const attrValue = node.getAttribute(attrName);
					if (attrValue && !isJsProtocol(attrValue)) {
						safeSendLink(attrValue, 'href');
					}
				});
			}
		}
	})();`

	// 触发事件和执行 JS 伪协议
	triggerEventsJS = `(function(){
		const treeWalker = document.createTreeWalker(
			document.documentElement,
			NodeFilter.SHOW_ELEMENT,
			{ acceptNode: () => NodeFilter.FILTER_ACCEPT }
		);

		const jsTasks = [];
		const eventList = [];
		
		while (treeWalker.nextNode()) {
			const node = treeWalker.currentNode;
			
			Array.from(node.attributes).forEach((attr) => {
				// 收集 JS 伪协议
				if (LINK_ATTRS.includes(attr.nodeName) && isJsProtocol(attr.nodeValue)) {
					jsTasks.push(() => safeEval(attr.nodeValue));
				}

				// 收集事件
				if (attr.nodeName.startsWith('on')) {
					// 内联事件
					eventList.push({name: attr.nodeName.substring(2), node: node});
				} else if (attr.nodeName === DOM_EVENT_FLAG) {
					// DOM 事件
					attr.nodeValue.split(',').forEach((eName) => {
						eventList.push({name: eName, node: node});
					});
				}
			});
		}

		// 将 JS 伪协议执行加入队列
		jsTasks.forEach(task => TaskQueue.add(task));

		// 将事件触发加入队列
		eventList.forEach((e) => {
			TaskQueue.add(() => {
				try {
					e.node.dispatchEvent(new Event(e.name, {bubbles: true}));
				} catch (err) {}
			});
		});
	})();`
)
