package main

const (
	bypassHeadlessDetectJS = `(function(w, n, wn) {
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

	initHookJS = `
		// hook 打开、关闭标签页函数
		window.open = function (url) {
			window.sendLink(JSON.stringify({url: new URL(url, document.baseURI).href, source: 'open'}));
		};
		window.close = function () {};

		// 锁定
		const actions = ['open', 'close'];
		actions.forEach(function (action) {
			Object.defineProperty(window, action, {
				writable: false, 
				configurable: false,
			});
		});

		// 劫持表单重置
		// 避免清空表单内容
		HTMLFormElement.prototype.reset = function() {};
		Object.defineProperty(HTMLFormElement.prototype, 'reset', {
			writable: false, 
			configurable: false,
		});

		const dom_event_flag = 'data-dom-events';

		// 记录 DOM0 事件注册
		// 即：使用 onXYZ 属性绑定方式注册事件
		// 使用属性的 setter 函数在注册事件时注入代码
		// 枚举 HTML DOM 事件: https://www.w3school.com.cn/jsref/dom_obj_event.asp
		const events = ['abort', 'afterprint', 'animationend', 'animationiteration', 'animationstart', 'beforeprint', 'beforeunload', 'blur', 'canplay', 'canplaythrough', 'change', 'click', 'contextmenu', 'copy', 'cut', 'dblclick', 'drag', 'dragend', 'dragenter', 'dragleave', 'dragover', 'dragstart', 'drop', 'durationchange', 'ended', 'error', 'focus', 'focusin', 'focusout', 'fullscreenchange', 'fullscreenerror', 'hashchange', 'input', 'invalid', 'keydown', 'keypress', 'keyup', 'load', 'loadeddata', 'loadedmetadata', 'loadstart', 'message', 'mousedown', 'mouseenter', 'mouseleave', 'mousemove', 'mouseout', 'mouseover', 'mouseup', 'mousewheel', 'offline', 'online', 'open', 'pagehide', 'pageshow', 'paste', 'pause', 'play', 'playing', 'popstate', 'progress', 'ratechange', 'reset', 'resize', 'scroll', 'search', 'seeked', 'seeking', 'select', 'show', 'stalled', 'storage', 'submit', 'suspend', 'timeupdate', 'toggle', 'touchcancel', 'touchend', 'touchmove', 'touchstart', 'transitionend', 'unload', 'volumechange', 'waiting', 'wheel'];

		events.forEach(function (eName) {
			Object.defineProperty(HTMLElement.prototype, 'on' + eName, {
				configurable: false,
				set: function(newValue){
					// 注入代码，给指定属性注册事件
					if (!this.hasAttribute(dom_event_flag)) {
						this.setAttribute(dom_event_flag, eName);
					} else {
						this.setAttribute(dom_event_flag, this.getAttribute(dom_event_flag) + ',' + eName);
					}
					// 保留原始逻辑
					// newValue 为绑定事件回调函数
					window['on' + eName] = newValue;
				}
			});
		});

		// 记录 DOM2 事件注册
		// 即：使用 addEventListener 方式注册事件
		let _addEventListener = Element.prototype.addEventListener;
		Element.prototype.addEventListener = function() {
			// 注入代码，给指定属性注册事件
			if (!this.hasAttribute(dom_event_flag)) {
				this.setAttribute(dom_event_flag, arguments[0]);
			} else {
				this.setAttribute(dom_event_flag, this.getAttribute(dom_event_flag) + ',' + arguments[0]);
			}
			// 保留原始逻辑
			_addEventListener.apply(this, arguments);
		};

		// 初始化工作
		// 可能包含链接的一些属性名
		const linkAttr = ['href', 'src', 'data-href', 'data-url', 'data-link'];

		// 随机选择数组中的一项
		function pickone(arr) {
			return arr[Math.floor(Math.random()*arr.length)];
		}

		// 从指定字符区间选择字符生成指定长度的字符串
		function random(chars, len) {
			let result = '';
			let counter = 0;
			while (counter < len) {
			  result += chars.charAt(Math.floor(Math.random() * chars.length));
			  counter += 1;
			}
			return result;
		}
	`

	mutationObserverJS = `(function(){
		let delay = 0;

		// 创建观察器实例
		let observer = new MutationObserver(function(mutations){
			mutations.forEach(function (mutation) {
				if (mutation.type === 'childList') {
					// 有新的节点
					for	(let i = 0; i < mutation.addedNodes.length; i++) {
						let	addedNode = mutation.addedNodes[i];
						if (addedNode.nodeType === Node.ELEMENT_NODE) {
							let aNodes = addedNode.getElementsByTagName('a');
							for (let j = 0; j < aNodes.length; j++) {
								let link = ''; 
								linkAttr.some(attr => (link = aNodes[j].getAttribute(attr)));
								if (link.toLowerCase().startsWith('javascript:')) {
									// 执行 javascript 代码
									try {
										if (!delay) {
											eval(link);
										} else {
											setTimeout(() => {
												eval(link);
											}, delay);
										}
										delay += 300;
									} catch (e) {}
								} else if (link) {
									// 记录链接
									window.sendLink(JSON.stringify({url: new URL(link, document.baseURI).href, source: 'dom'}));
								}
							}
						}
					}
				} else if (mutation.type === 'attributes') {
					// 属性发生变化
					if (mutation.target.nodeType === Node.ELEMENT_NODE) {
						if (linkAttr.includes(mutation.attributeName)) {
							let link = mutation.target.getAttribute(mutation.attributeName);
							if (link.toLowerCase().startsWith('javascript:')) {
								// 执行 javascript 代码
								try {
									if (!delay) {
										eval(link);
									} else {
										setTimeout(() => {
											eval(link);
										}, delay);
									}
									delay += 300;
								} catch (e) {}
							} else {
								// 记录链接
								window.sendLink(JSON.stringify({url: new URL(link, document.baseURI).href, source: 'dom'}));
							}
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
			attributeFilter: linkAttr
		});
	})();`

	// 遍历元素，收集链接和事件，并触发事件
	collectLinksAndEventsJS = `(function(){
		let delay = 0;

		let treeWalker = document.createTreeWalker(
			document.documentElement,
			NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_COMMENT,
			{ acceptNode(node) { return NodeFilter.FILTER_ACCEPT; } }
		);

		// 检测注释里的完整 URL
		const urlRe = /https?:\/\/(?:www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b(?:[-a-zA-Z0-9()@:%_\+.~#?&\/=]*)/g;
		let eList = [];
		
		while(treeWalker.nextNode()) {
			let cNode = treeWalker.currentNode;
			if (cNode.nodeType === Node.COMMENT_NODE) {
				// 注释节点
				let match;
				while ((match = urlRe.exec(cNode.nodeValue)) !== null) {
					// 记录链接
					window.sendLink(JSON.stringify({url: new URL(match[0], document.baseURI).href, source: 'comment'}));
				}
			} else {
				for(let i = 0; i < cNode.attributes.length; i++) {
					let attr = cNode.attributes[i];

					// 收集链接
					if (linkAttr.includes(attr.nodeName)) {
						if (attr.nodeValue.toLowerCase().startsWith('javascript:')) {
							// 执行 javascript 代码
							try {
								if (!delay) {
									eval(attr.nodeValue);
								} else {
									setTimeout(() => {
										eval(attr.nodeValue);
									}, delay);
								}
								delay += 5000;
							} catch (e) {}
						} else {
							// 记录链接
							window.sendLink(JSON.stringify({url: new URL(attr.nodeValue, document.baseURI).href, source: 'href'}));
						}
					}

					// 收集事件
					if (attr.nodeName.startsWith('on')) {
						// 内联事件
						eList.push({"ename": attr.nodeName.substring(2), "cnode": cNode});
					} else if (attr.nodeName === dom_event_flag) {
						// DOM 事件
						let eArr = attr.nodeValue.split(',');
						eArr.forEach(function(eName) {
							eList.push({"ename": eName, "cnode": cNode});
						});
					}
				}
			}
		};

		// 触发事件
		eList.forEach(function (e) {
			let event = new Event(e.ename);
			try {
				if (!delay) {
					e.cnode.dispatchEvent(event);
				} else {
					setTimeout(() => {
						e.cnode.dispatchEvent(event);
					}, delay);
				}
				delay += 5000;
			} catch (err) {}
		});
	})();`

	// 填充和提交表单
	fillAndSubmitFormsJS = `(function(){
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

		let corpus = {
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

		// 创建隐藏的表单提交 target
		let iframe = document.createElement('iframe');
		iframe.style.display = 'none';
		iframe.name = 'thiis_is_a_iframe_7';
		document.body.appendChild(iframe);
		
		let forms = [];
		// 表单元素 select 默认选择第一个 option，暂不处理
		// 填充 input 和 textarea
		for (let i = 0; i < document.forms.length; i++) {
			let form = document.forms[i];
			// 将表单 target 指向 iframe
			form.setAttribute('target', 'thiis_is_a_iframe_7');
			for(let j = 0; j < form.length; j++) {
				let ele = form[j];
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
						let year = pickone(corpus.year);
						let month = pickone(corpus.month);
						let day = pickone(corpus.day);
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
			forms.push(form);
		}

		// 提交表单
		forms.forEach(function(form) {
			try {
				form.submit();
			} catch(e) {
				for(let i = 0; i < form.length; i++) {
        			if (form[i].nodeName == 'INPUT') {
        				if (form[i].type == 'submit' || form[i].type == 'button') {
        					// 点击提交按钮或其它按钮
        					try {
        						form[i].click();
        					} catch (e) {}
        				}
        			} else if (form[i].nodeName == 'BUTTON') {
        				// 点击按钮
        				try {
        					form[i].click();
        				} catch (e) {}
        			}
        		}
			}
		});
	})();`
)
