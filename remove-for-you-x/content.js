function removeForYouTab() {
  const selectors = [
    'a[href="/home"]',
    'a[data-testid="AppTabBar_Home_Link"]',
    '[data-testid="primaryNavigation"] a[href="/home"]',
    'nav a[href="/home"]',
    'a[aria-label*="Home"]'
  ];
  
  selectors.forEach(selector => {
    const elements = document.querySelectorAll(selector);
    elements.forEach(element => {
      const text = element.textContent?.toLowerCase();
      if (text?.includes('for you') || text?.includes('home')) {
        element.style.display = 'none';
        element.remove();
      }
    });
  });
}

function observeAndRemove() {
  removeForYouTab();
  
  const observer = new MutationObserver((mutations) => {
    mutations.forEach((mutation) => {
      if (mutation.type === 'childList') {
        removeForYouTab();
      }
    });
  });
  
  observer.observe(document.body, {
    childList: true,
    subtree: true
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', observeAndRemove);
} else {
  observeAndRemove();
}

setInterval(removeForYouTab, 1000);
