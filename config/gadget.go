package config

const (
	BypassHeadlessDetectJS = `(function(w, n, wn) {
		// Pass the Webdriver Test.
		Object.defineProperty(n, 'webdriver', {
		  get: () => false,
		});

		// Pass the Plugins Length Test.
		// Overwrite the plugins property to use a custom getter.
		Object.defineProperty(n, 'plugins', {
		  // This just needs to have length > 0 for the current test,
		  // but we could mock the plugins too if necessary.
		  get: () => [1, 2, 3, 4, 5],
		});

		// Pass the Languages Test.
		// Overwrite the plugins property to use a custom getter.
		Object.defineProperty(n, 'languages', {
		  get: () => ['zh-CN', 'zh'],
		});

		// Pass the Chrome Test.
		// We can mock this in as much depth as we need for the test.
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

	InitHookJS = `(function () {
		// 劫持打开标签页
		window.open = function (url) {
			window.sendLink(JSON.stringify({url: new URL(url, document.baseURI).href, source: 'open'}));
		}
		Object.defineProperty(window, 'open', {
			writable: false, 
			configurable: false
		});
		
		// 劫持关闭标签页
		window.close = function() {};
		Object.defineProperty(window, 'close', {
			writable: false, 
			configurable: false
		});
		
		// 劫持 fetch 请求
		var oldFetch = window.fetch;
		window.fetch = function(url, init) {
			// hook code
			return oldFetch(url, init);
		};

		// 劫持 xhr 请求
		XMLHttpRequest.prototype.__originalOpen = XMLHttpRequest.prototype.open;
		XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
			// hook code
			return this.__originalOpen(method, url, async, user, password);
		};
		Object.defineProperty(XMLHttpRequest.prototype, 'open', {
			writable: false, 
			configurable: false
		});
		
		XMLHttpRequest.prototype.__originalSend = XMLHttpRequest.prototype.send;
		XMLHttpRequest.prototype.send = function(data) {
			// hook code
			return this.__originalSend(data);
		};
		Object.defineProperty(XMLHttpRequest.prototype, 'send', {
			writable: false, 
			configurable: false
		});

		XMLHttpRequest.prototype.__originalAbort = XMLHttpRequest.prototype.abort;
		XMLHttpRequest.prototype.abort = function() {
			// hook code
		};
		Object.defineProperty(XMLHttpRequest.prototype, 'abort', {
			writable: false, 
			configurable: false
		});

		// 劫持 timer 和 ticker
		window.__originalSetTimeout = window.setTimeout;
		window.setTimeout = function() {
		    arguments[1] = 0;
		    return window.__originalSetTimeout.apply(this, arguments);
		};
		Object.defineProperty(window, 'setTimeout', {
			writable: false, 
			configurable: false
		});

		// ticker 只执行一次
		window.setInterval = function() {
			arguments[1] = 1000;
			return window.__originalSetTimeout.apply(this, arguments);
		};
		Object.defineProperty(window, 'setInterval', {
			writable: false, 
			configurable: false
		});

		// 劫持表单重置
		HTMLFormElement.prototype.reset = function() {};
		Object.defineProperty(HTMLFormElement.prototype, 'reset', {
			writable: false, 
			configurable: false
		});

		var dom_event_flag = 'data-dom-events';

		// 记录 DOM0 事件注册
		// 即：使用 onXYZ 属性绑定方式注册事件
		function injectDOM0(obj, eventName) {
			if (!obj.hasAttribute(dom_event_flag)) {
				obj.setAttribute(dom_event_flag, eventName);
			} else {
				obj.setAttribute(dom_event_flag, obj.getAttribute(dom_event_flag) + ',' + eventName);
			}
		}

		// 使用属性的 setter 函数在注册事件时注入代码
		// 枚举 HTML DOM 事件: https://www.w3school.com.cn/jsref/dom_obj_event.asp
		var events = ['abort', 'afterprint', 'animationend', 'animationiteration', 'animationstart', 'beforeprint', 'beforeunload', 'blur', 'canplay', 'canplaythrough', 'change', 'click', 'contextmenu', 'copy', 'cut', 'dblclick', 'drag', 'dragend', 'dragenter', 'dragleave', 'dragover', 'dragstart', 'drop', 'durationchange', 'ended', 'error', 'focus', 'focusin', 'focusout', 'fullscreenchange', 'fullscreenerror', 'hashchange', 'input', 'invalid', 'keydown', 'keypress', 'keyup', 'load', 'loadeddata', 'loadedmetadata', 'loadstart', 'message', 'mousedown', 'mouseenter', 'mouseleave', 'mousemove', 'mouseout', 'mouseover', 'mouseup', 'mousewheel', 'offline', 'online', 'open', 'pagehide', 'pageshow', 'paste', 'pause', 'play', 'playing', 'popstate', 'progress', 'ratechange', 'reset', 'resize', 'scroll', 'search', 'seeked', 'seeking', 'select', 'show', 'stalled', 'storage', 'submit', 'suspend', 'timeupdate', 'toggle', 'touchcancel', 'touchend', 'touchmove', 'touchstart', 'transitionend', 'unload', 'volumechange', 'waiting', 'wheel'];

		events.forEach(function (eventName) {
			Object.defineProperty(HTMLElement.prototype, 'on' + eventName, {
				configurable: false,
				set: function(newValue){
					injectDOM0(this, eventName);
					// newValue 为绑定事件回调函数
					window['on' + eventName] = newValue;
				}
			});
		});

		// 记录 DOM2 事件注册
		// 即：使用 addEventListener 方式注册事件
		var _addEventListener = Element.prototype.addEventListener;
		Element.prototype.addEventListener = function() {
			if (!this.hasAttribute(dom_event_flag)) {
				this.setAttribute(dom_event_flag, arguments[0]);
			} else {
				this.setAttribute(dom_event_flag, this.getAttribute(dom_event_flag) + ',' + arguments[0]);
			}
			_addEventListener.apply(this, arguments);
		};
	})();`

	MutationObserverJS = `(function(){
		// 创建观察器实例
		var observer = new MutationObserver(function(mutations){
			mutations.forEach(function (mutation) {
				if (mutation.type === 'childList') {
					// 有新的节点
					for	(var i = 0; i < mutation.addedNodes.length; i++) {
						var	addedNode = mutation.addedNodes[i];
						if (addedNode.nodeType === Node.ELEMENT_NODE) {
							var link = addedNode.getAttribute('href') || addedNode.getAttribute('src') || '';
							if (!link.toLowerCase().startsWith('javascript:')) {
								// 记录链接
								window.sendLink(JSON.stringify({url: new URL(link, document.baseURI).href, source: 'DOM'}));
							} else {
								// 执行 javascript 代码
								try {
									eval(link.substring(11));
								} catch (e) {}
							}
						}
					}
				} else if (mutation.type === 'attributes') {
					// 属性发生变化
					var target = mutation.target;
					if (target.nodeType === Node.ELEMENT_NODE) {
						var link = target.getAttribute(mutation.attributeName);
						if (!link.toLowerCase().startsWith('javascript:')) {
							// 记录链接
							window.sendLink(JSON.stringify({url: new URL(link, document.baseURI).href, source: 'DOM'}));
						} else {
							// 执行 javascript 代码
							try {
								eval(link.substring(11));
							} catch (e) {}
						}
					}
				}
			});
		});
	
		// 启动观察
		observer.observe(document.documentElement, {
			subtree: true,
			childList: true,
			attributes: true,
			attributeFilter: ['src', 'href']
		});
	})();`

	// 遍历元素，收集链接
	CollectLinksJS = `(function(){
		var treeWalker = document.createTreeWalker(
			document.documentElement,
			NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_COMMENT,
			{ acceptNode(node) { return NodeFilter.FILTER_ACCEPT; } }
		);

		// 检测注释里的完整 URL
		var urlRe = /https?:\/\/(?:www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b(?:[-a-zA-Z0-9()@:%_\+.~#?&\/=]*)/g;
		var linkAttr = ['src', 'href', 'data-href', 'data-url', 'data-link'];
		var currentNode = treeWalker.currentNode;
		
		while(currentNode) {
			if (currentNode.nodeType === Node.COMMENT_NODE) {
				// 注释节点
				do {
					var url = urlRe.exec(currentNode.nodeValue);
					if (url) {
						// 记录链接
						window.sendLink(JSON.stringify({url: new URL(url[0], document.baseURI).href, source: 'comment'}));
					}
				} while (url);
			} else {
				for(var i = 0; i < currentNode.attributes.length; i++) {
					var attr = currentNode.attributes[i];
					if (linkAttr.includes(attr.nodeName)) {
						if (!attr.nodeValue.toLowerCase().startsWith('javascript:')) {
							// 记录链接
							window.sendLink(JSON.stringify({url: new URL(attr.nodeValue, document.baseURI).href, source: 'DOM'}));
						} else {
							// 执行 javascript 代码
							try {
								eval(attr.nodeValue.substring(11));
							} catch (e) {}
						}
					}
				}
			}
  			currentNode = treeWalker.nextNode();
		};
	})();`

	// 收集事件并触发
	// 内联事件
	CollectAndTriggerInlineEventJS = `(function(){
		// 收集
		var treeWalker = document.createTreeWalker(
			document.body,
			NodeFilter.SHOW_ELEMENT,
			{ acceptNode(node) { return NodeFilter.FILTER_ACCEPT; } }
		);

		var inlineList = [];
		var currentNode = treeWalker.currentNode;
		
		while(currentNode) {
			for(var i = 0; i < currentNode.attributes.length; i++) {
				var attr = currentNode.attributes[i];
				if (attr.nodeName.startsWith('on')) {
					inlineList.push({key: attr.nodeName.substring(2), value: currentNode});
				}
			}
  			currentNode = treeWalker.nextNode();
		};

		// 触发
		inlineList.forEach(function (item) {
			var ev = new Event(item.key);
			try {
				item.value.dispatchEvent(ev);
			} catch (e) {}
		});
	})();`

	// DOM 事件
	CollectAndTriggerDOMEventJS = `(function(){
		// 收集
		var treeWalker = document.createTreeWalker(
			document.body,
			NodeFilter.SHOW_ELEMENT,
			{ acceptNode(node) { return NodeFilter.FILTER_ACCEPT; } }
		);

		var domList = [];
		var currentNode = treeWalker.currentNode;
		var dom_event_flag = 'data-dom-events';
		
		while(currentNode) {
			for(var i = 0; i < currentNode.attributes.length; i++) {
				var attr = currentNode.attributes[i];
				if (attr.nodeName === dom_event_flag) {
					var eventArr = attr.nodeValue.split(',');
					eventArr.forEach(function(eventName) {
						domList.push({key: eventName, value: currentNode});
					});
				}
			}
  			currentNode = treeWalker.nextNode();
		};

		// 触发
		domList.forEach(function (item) {
			var ev = new Event(item.key);
			try {
				item.value.dispatchEvent(ev);
			} catch (e) {}
		});
	})();`

	// 填充和提交表单
	FillAndSubmitFormsJS = `(function(){
		// 创建隐藏的表单提交 target
		var iframe = document.createElement('iframe');
		iframe.style.display = 'none';
		iframe.name = 'thiis_is_a_iframe_7';
		document.body.appendChild(iframe);

		/*
		1. 填充表单
			1-1. 遍历表单元素 (input, select, textarea)
			1-2. 识别输入类型
				- text
				- password
				- radio
				- checkbox
				- color
				- date
				- datetime-local
				- email
				- month
				- number
				- range
				- search
				- time
				- url
				- week
			1-3. 按类型和name属性值填充表单
		2. 提交表单
			2-1. 点击提交按钮 (<input type="submit">)
			2-2. JS 提交表单 (form.submit())
			2-3. 点击其他按钮
				- <button>
				- <input type="button">
		*/

		function random(characters, length) {
			var result = '';
			var charactersLength = characters.length;
			var counter = 0;
			while (counter < length) {
			  result += characters.charAt(Math.floor(Math.random() * charactersLength));
			  counter += 1;
			}
			return result;
		}

		function pickone(arr) {
			return arr[Math.floor(Math.random()*arr.length)];
		}

		var corpus = {
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
		
		var forms = [];
		// 表单元素 select 默认选择第一个 option，暂不处理
		// 填充 input 和 textarea
		for (var i = 0; i < document.forms.length; i++) {
			var form = document.forms[i];
			// 将表单 target 指向 iframe
			form.setAttribute('target', 'thiis_is_a_iframe_7');
			forms.push(form);
			for(var j = 0; j < form.length; j++) {
				var ele = form[j];
				if (ele.nodeName == 'INPUT') {
					if (ele.type == 'text') {
						if (/((number)|(phone))|(^tel)/i.test(ele.name)) {
							// 手机
							ele.value = '139' + random(corpus.digit, 8);
						} else if (/mail|email/i.test(ele.name)) {
							// 邮箱
							ele.value = pickone(corpus.firstname) + '.' + pickone(corpus.lastname) + '@' + random(corpus.digit, 5) + pickone(corpus.domain);
						} else if (/url|website|blog|homepage/i.test(ele.name)) {
							// 主页
							ele.value = 'https://www.' + random(corpus.digit, 5) + pickone(corpus.domain);
						} else if (/(date)|(^birth)/i.test(ele.name)) {
							// 生日
							ele.value = pickone(corpus.year) + pickone(corpus.month) + pickone(corpus.day);
						} else if (/^addr/i.test(ele.name)) {
							// 地址
							ele.value = pickone(corpus.address);
						} else {
							ele.value = 'flamingo';
						}
					} else if (ele.type == 'password') {
						ele.value = random(corpus.letter, 4) + random(corpus.symbol, 2) + random(corpus.digit, 4);
					} else if (ele.type == 'radio' || ele.type == 'checkbox') {
						ele.checked = true;
					} else if (ele.type == 'month' || ele.type == 'week' || ele.type == 'date' || ele.type == 'datetime-local' || ele.type == 'time') {
						var year = pickone(corpus.year);
						var month = pickone(corpus.month);
						var day = pickone(corpus.day);
						if (ele.type == 'month') {
							ele.value = year + '-' + month;
						} else if (ele.type == 'week') {
							ele.value = year + '-W10';
						} else if (ele.type == 'date') {
							ele.value = year + '-' + month + '-' + day;
						} else if (ele.type == 'datetime-local') {
							ele.value = year + '-' + month + '-' + day + ' 10:00';
						} else {
							ele.value = '10:00';
						}
					} else if (ele.type == 'email') {
						ele.value = pickone(corpus.firstname) + '.' + pickone(corpus.lastname) + '@' + random(corpus.digit, 5) + pickone(corpus.domain);
					} else if (ele.type == 'number' || ele.type == 'range') {
						if (ele.hasAttribute('min') && ele.hasAttribute('max')) {
							ele.value = Math.floor(Math.random() * (ele.max - ele.min + 1) + ele.min);
						} else if (ele.hasAttribute('min')) {
							ele.value = ele.min + 1;
						} else if (ele.hasAttribute('max')) {
							ele.value = ele.max - 1;
						} else {
							ele.value = random(corpus.digit, 1);
						}
					} else if (ele.type == 'search') {
						ele.value = 'flamingo';
					} else if (ele.type == 'url') {
						ele.value = 'https://www.' + random(corpus.digit, 5) + pickone(corpus.domain);
					}
				} else if (ele.nodeName == 'TEXTAREA') {
					ele.value = 'tested by flamingo';
				}
			}
		}

		// 提交表单
		forms.forEach(function(form) {
			// 直接提交
			try {
				form.submit();
			} catch (e) {}
		});

		for(var i = 0; i < form.length; i++) {
			var ele = form[j];
			if (ele.nodeName == 'INPUT') {
				if (ele.type == 'submit' || ele.type == 'button') {
					// 点击提交按钮或其它按钮
					try {
						ele.click();
					} catch (e) {}
					
				}
			} else if (ele.nodeName == 'BUTTON') {
				// 点击按钮
				try {
					ele.click();
				} catch (e) {}
			}
		}
	})()`
)
