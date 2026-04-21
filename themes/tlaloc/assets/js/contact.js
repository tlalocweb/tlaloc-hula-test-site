(function () {
  'use strict';

  var API_URL = typeof window.__TLALOC_API === 'string' ? window.__TLALOC_API : '';
  var EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  var CODE_RE = /^[0-9]{6}$/;

  var modal, backdrop, closeBtn;
  var stepForm, stepVerify, stepSuccess;
  var contactForm, verifyForm;
  var nameInput, emailInput, messageInput, codeInput;
  var nameError, emailError, messageError, turnstileError, formError, codeError, verifyError;
  var submitBtn, verifyBtn;
  var emailDisplay;
  var turnstileWidgetId = null;
  var submittedEmail = '';
  var browserId = String(navigator.userAgent) + '|' + screen.width + 'x' + screen.height + '|' + new Date().getTimezoneOffset();

  function init() {
    modal = document.getElementById('contact-modal');
    backdrop = document.getElementById('modal-backdrop');
    closeBtn = document.getElementById('modal-close-btn');

    stepForm = document.getElementById('step-form');
    stepVerify = document.getElementById('step-verify');
    stepSuccess = document.getElementById('step-success');

    contactForm = document.getElementById('contact-form');
    verifyForm = document.getElementById('verify-form');

    nameInput = document.getElementById('contact-name');
    emailInput = document.getElementById('contact-email');
    messageInput = document.getElementById('contact-message');
    codeInput = document.getElementById('verify-code');

    nameError = document.getElementById('name-error');
    emailError = document.getElementById('email-error');
    messageError = document.getElementById('message-error');
    turnstileError = document.getElementById('turnstile-error');
    formError = document.getElementById('form-error');
    codeError = document.getElementById('code-error');
    verifyError = document.getElementById('verify-error');

    submitBtn = document.getElementById('contact-submit-btn');
    verifyBtn = document.getElementById('verify-submit-btn');
    emailDisplay = document.getElementById('verify-email-display');

    document.getElementById('contact-open-btn').addEventListener('click', openModal);
    closeBtn.addEventListener('click', closeModal);
    backdrop.addEventListener('click', closeModal);
    document.getElementById('contact-cancel-btn').addEventListener('click', closeModal);
    document.getElementById('verify-back-btn').addEventListener('click', goBackToForm);
    document.getElementById('success-close-btn').addEventListener('click', closeModal);

    contactForm.addEventListener('submit', onContactSubmit);
    verifyForm.addEventListener('submit', onVerifySubmit);

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && modal.getAttribute('aria-hidden') === 'false') {
        closeModal();
      }
    });
  }

  function openModal() {
    modal.setAttribute('aria-hidden', 'false');
    modal.classList.add('modal--open');
    document.body.style.overflow = 'hidden';
    showStep('form');
    renderTurnstile();
    nameInput.focus();
  }

  function closeModal() {
    modal.setAttribute('aria-hidden', 'true');
    modal.classList.remove('modal--open');
    document.body.style.overflow = '';
    resetForms();
    removeTurnstile();
  }

  function showStep(step) {
    stepForm.classList.toggle('modal__step--hidden', step !== 'form');
    stepVerify.classList.toggle('modal__step--hidden', step !== 'verify');
    stepSuccess.classList.toggle('modal__step--hidden', step !== 'success');
  }

  function goBackToForm() {
    showStep('form');
    clearErrors();
  }

  function resetForms() {
    contactForm.reset();
    verifyForm.reset();
    clearErrors();
    submitBtn.disabled = false;
    verifyBtn.disabled = false;
    submittedEmail = '';
  }

  function clearErrors() {
    var errors = [nameError, emailError, messageError, turnstileError, formError, codeError, verifyError];
    for (var i = 0; i < errors.length; i++) {
      errors[i].textContent = '';
    }
    var invalids = modal.querySelectorAll('.form__input--invalid');
    for (var j = 0; j < invalids.length; j++) {
      invalids[j].classList.remove('form__input--invalid');
    }
  }

  function renderTurnstile() {
    if (typeof turnstile === 'undefined') return;
    var container = document.getElementById('turnstile-container');
    if (turnstileWidgetId !== null) {
      turnstile.reset(turnstileWidgetId);
      return;
    }
    var sitekey = container.closest('[data-turnstile-sitekey]');
    var key = sitekey ? sitekey.getAttribute('data-turnstile-sitekey') : '';
    if (!key) {
      var meta = document.querySelector('meta[name="turnstile-sitekey"]');
      key = meta ? meta.getAttribute('content') : '';
    }
    if (!key) return;
    turnstileWidgetId = turnstile.render(container, { sitekey: key });
  }

  function removeTurnstile() {
    if (typeof turnstile !== 'undefined' && turnstileWidgetId !== null) {
      turnstile.remove(turnstileWidgetId);
      turnstileWidgetId = null;
    }
  }

  function getTurnstileToken() {
    if (typeof turnstile === 'undefined' || turnstileWidgetId === null) return '';
    return turnstile.getResponse(turnstileWidgetId) || '';
  }

  function validateContactForm() {
    clearErrors();
    var valid = true;

    if (nameInput.value.trim() === '') {
      nameError.textContent = 'Name is required';
      nameInput.classList.add('form__input--invalid');
      valid = false;
    }

    var email = emailInput.value.trim();
    if (email === '') {
      emailError.textContent = 'Email is required';
      emailInput.classList.add('form__input--invalid');
      valid = false;
    } else if (!EMAIL_RE.test(email)) {
      emailError.textContent = 'Please enter a valid email';
      emailInput.classList.add('form__input--invalid');
      valid = false;
    }

    if (messageInput.value.trim() === '') {
      messageError.textContent = 'Please tell us how we can help';
      messageInput.classList.add('form__input--invalid');
      valid = false;
    }

    var token = getTurnstileToken();
    if (!token) {
      turnstileError.textContent = 'Please complete the security check';
      valid = false;
    }

    return valid;
  }

  function onContactSubmit(e) {
    e.preventDefault();
    if (!validateContactForm()) return;

    submitBtn.disabled = true;
    formError.textContent = '';

    var data = {
      name: nameInput.value.trim(),
      email: emailInput.value.trim(),
      message: messageInput.value.trim(),
      turnstileToken: getTurnstileToken(),
      browserId: browserId
    };

    submittedEmail = data.email;

    fetch(API_URL + '/api/contact', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data)
    })
    .then(function (res) {
      return res.json().then(function (body) {
        return { ok: res.ok, data: body };
      });
    })
    .then(function (result) {
      if (result.data.label === 'real_interest') {
        emailDisplay.textContent = submittedEmail;
        showStep('verify');
        codeInput.focus();
      } else if (result.data.label === 'spam' || result.data.label === 'sales_pitch') {
        formError.textContent = result.data.message || 'Please adjust your message.';
        submitBtn.disabled = false;
        if (typeof turnstile !== 'undefined' && turnstileWidgetId !== null) {
          turnstile.reset(turnstileWidgetId);
        }
      } else if (!result.ok) {
        formError.textContent = result.data.error || 'Something went wrong.';
        submitBtn.disabled = false;
        if (typeof turnstile !== 'undefined' && turnstileWidgetId !== null) {
          turnstile.reset(turnstileWidgetId);
        }
      }
    })
    .catch(function () {
      formError.textContent = 'Network error. Please try again.';
      submitBtn.disabled = false;
    });
  }

  function onVerifySubmit(e) {
    e.preventDefault();
    clearErrors();

    var code = codeInput.value.trim();
    if (!CODE_RE.test(code)) {
      codeError.textContent = 'Please enter a 6-digit code';
      codeInput.classList.add('form__input--invalid');
      return;
    }

    verifyBtn.disabled = true;
    verifyError.textContent = '';

    fetch(API_URL + '/api/verify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: submittedEmail, code: code })
    })
    .then(function (res) {
      return res.json().then(function (body) {
        return { ok: res.ok, data: body };
      });
    })
    .then(function (result) {
      if (result.ok) {
        showStep('success');
      } else {
        verifyError.textContent = result.data.error || 'Invalid code.';
        verifyBtn.disabled = false;
      }
    })
    .catch(function () {
      verifyError.textContent = 'Network error. Please try again.';
      verifyBtn.disabled = false;
    });
  }

  document.addEventListener('DOMContentLoaded', init);
})();
