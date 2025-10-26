/**
 * Name Section Component
 * Handles auto-generation of slugified name from display_name
 */

(function() {
  'use strict';

  /**
   * Slugify function - converts text to URL-friendly slug
   */
  const slugify = (text) => {
    return text
      .toString()                   // Cast to string (optional)
      .normalize('NFKD')            // The normalize() using NFKD method returns the Unicode Normalization Form of a given string.
      .toLowerCase()                // Convert the string to lowercase letters
      .trim()                       // Remove whitespace from both sides of a string (optional)
      .replace(/\s+/g, '-')         // Replace spaces with -
      .replace(/[^\w\-]+/g, '')     // Remove all non-word chars
      .replace(/\_/g,'-')           // Replace _ with -
      .replace(/\-\-+/g, '-')       // Replace multiple - with single -
      .replace(/\-$/g, '');         // Remove trailing -
  };

  /**
   * Initialize all name sections on the page
   */
  function initNameSections() {
    const sections = document.querySelectorAll('[data-name-section]');
    sections.forEach(section => {
      if (!section.dataset.nameSectionInitialized) {
        new NameSection(section);
      }
    });
  }

  /**
   * NameSection class
   */
  class NameSection {
    constructor(element) {
      this.element = element;
      this.element.dataset.nameSectionInitialized = 'true';
      
      this.displayInput = element.querySelector('[data-name-display-input]');
      this.nameInput = element.querySelector('[data-name-input]');
      this.nameFieldWrapper = element.querySelector('[data-name-field-wrapper]');
      this.nameDescription = element.querySelector('[data-name-description]');
      this.nameDisplayText = element.querySelector('[data-name-display-text]');
      this.nameValue = element.querySelector('[data-name-value]');
      this.editButton = element.querySelector('[data-name-edit-button]');

      if (!this.displayInput || !this.nameInput) {
        console.error('NameSection: Missing required elements');
        return;
      }

      // Check if name already has a value (editing mode)
      this.editMode = this.nameInput.value.trim() !== '';
      
      this.setupEventListeners();
      this.updateUI();
    }

    setupEventListeners() {
      // Listen for input changes on display_name
      this.displayInput.addEventListener('input', () => {
        if (!this.editMode) {
          this.updateNameFromDisplay();
        }
      });

      // Listen for edit button clicks
      if (this.editButton) {
        this.editButton.addEventListener('click', (e) => {
          e.preventDefault();
          this.enableEditMode();
        });
      }
    }

    updateNameFromDisplay() {
      const displayValue = this.displayInput.value.trim();
      const slugifiedName = slugify(displayValue);
      
      this.nameInput.value = slugifiedName;
      this.updateDisplay();
    }

    updateDisplay() {
      const nameValue = this.nameInput.value.trim();
      
      if (nameValue) {
        if (this.nameValue) {
          this.nameValue.textContent = nameValue;
        }
        if (this.nameDisplayText) {
          this.nameDisplayText.style.display = '';
        }
      } else {
        if (this.nameDisplayText) {
          this.nameDisplayText.style.display = 'none';
        }
      }
    }

    enableEditMode() {
      this.editMode = true;
      this.updateUI();
      
      // Focus the name input
      if (this.nameInput) {
        this.nameInput.focus();
      }
    }

    updateUI() {
      if (this.editMode) {
        // Show name input field, hide description
        if (this.nameFieldWrapper) {
          this.nameFieldWrapper.style.display = '';
        }
        if (this.nameDescription) {
          this.nameDescription.style.display = 'none';
        }
      } else {
        // Show description, hide name input field
        if (this.nameFieldWrapper) {
          this.nameFieldWrapper.style.display = 'none';
        }
        if (this.nameDescription) {
          this.nameDescription.style.display = '';
        }
        
        // Update the displayed name value
        this.updateDisplay();
      }
    }
  }

  // Initialize on DOMContentLoaded
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initNameSections);
  } else {
    initNameSections();
  }

  // Re-initialize when new content is added (for dynamic content)
  if (window.MutationObserver) {
    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        mutation.addedNodes.forEach((node) => {
          if (node.nodeType === 1) { // Element node
            if (node.matches('[data-name-section]')) {
              new NameSection(node);
            } else {
              const sections = node.querySelectorAll('[data-name-section]');
              sections.forEach(section => {
                if (!section.dataset.nameSectionInitialized) {
                  new NameSection(section);
                }
              });
            }
          }
        });
      });
    });

    observer.observe(document.body, {
      childList: true,
      subtree: true
    });
  }

  // Export for testing
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { NameSection, slugify };
  }
})();
