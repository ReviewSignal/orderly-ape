/**
 * EditableList Component
 * Handles add/edit/delete operations for list items with JSON aggregation
 */

(function() {
  'use strict';

  /**
   * Initialize all editable lists on the page
   */
  function initEditableLists() {
    const lists = document.querySelectorAll('[data-editable-list-name]');
    lists.forEach(list => {
      if (!list.dataset.editableListInitialized) {
        new EditableList(list);
      }
    });
  }

  /**
   * EditableList class
   */
  class EditableList {
    constructor(element) {
      this.element = element;
      this.element.dataset.editableListInitialized = 'true';
      this.name = element.dataset.editableListName;
      this.disabled = element.dataset.editableListDisabled === 'true';
      this.required = element.dataset.editableListRequired === 'true';
      this.readonly = element.dataset.editableListReadonly === 'true';
      this.maxItems = element.dataset.editableListMaxItems ? parseInt(element.dataset.editableListMaxItems, 10) : 0;
      this.extraItems = element.dataset.editableListExtraItems ? parseInt(element.dataset.editableListExtraItems, 10) : 0;
      this.hiddenInput = element.querySelector('[data-editable-list-hidden-input]');
      this.itemsContainer = element.querySelector('[data-editable-list-items]');
      this.template = element.querySelector('[data-editable-list-template]');
      this.addButton = element.querySelector('[data-editable-list-add]');

      if (!this.hiddenInput || !this.itemsContainer || !this.template) {
        console.error('EditableList: Missing required elements');
        return;
      }

      this.items = [];
      this.setupEventListeners();
      this.initializeExistingItems();
    }

    setupEventListeners() {
      if (this.addButton) {
        this.addButton.addEventListener('click', () => this.addItem());
      }

      // Delegate event listeners for dynamically added items
      this.itemsContainer.addEventListener('click', (e) => {
        const deleteBtn = e.target.closest('[data-editable-list-item-delete]');
        if (deleteBtn) {
          const item = deleteBtn.closest('[data-editable-list-item]');
          if (item) {
            this.deleteItem(item);
          }
        }
      });

      // Listen for input changes to update JSON
      this.itemsContainer.addEventListener('input', () => {
        this.updateHiddenInput();
      });

      this.itemsContainer.addEventListener('change', () => {
        this.updateHiddenInput();
      });
    }

    initializeExistingItems() {
      // If there are pre-existing items in the container, register them
      const existingItems = this.itemsContainer.querySelectorAll('[data-editable-list-item]');
      existingItems.forEach(item => {
        this.items.push(item);
      });

      // Check if there's an initial value in the hidden input
      if (this.items.length === 0 && this.hiddenInput.value) {
        try {
          const initialData = JSON.parse(this.hiddenInput.value);
          this.populateFromData(initialData);
        } catch (e) {
          console.warn('EditableList: Failed to parse initial value', e);
          // Add items based on extraItems if parsing failed
          const itemsToAdd = this.extraItems > 0 ? this.extraItems : 1;
          for (let i = 0; i < itemsToAdd; i++) {
            this.addItem();
          }
        }
      } else if (this.items.length === 0) {
        // Add items based on extraItems setting
        const itemsToAdd = this.extraItems > 0 ? this.extraItems : 0;
        for (let i = 0; i < itemsToAdd; i++) {
          this.addItem();
        }
      } else {
        // Update hidden input with existing items
        this.updateHiddenInput();
      }

      // Update button visibility based on max items
      this.updateAddButtonVisibility();
    }

    addItem() {
      if (this.disabled || this.readonly) return;

      // Check max items limit
      if (this.maxItems > 0 && this.items.length >= this.maxItems) {
        return;
      }

      // Clone the template content
      const templateContent = this.template.content.cloneNode(true);
      const item = templateContent.querySelector('[data-editable-list-item]');

      if (!item) {
        console.error('EditableList: No item found in template');
        return;
      }

      // Generate unique ID for the new item
      item.id = 'list-item-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);

      this.itemsContainer.appendChild(templateContent);
      this.items.push(item);
      
      // Apply readonly state if needed
      if (this.readonly) {
        this.setItemReadonly(item);
      }
      
      this.updateHiddenInput();
      this.updateAddButtonVisibility();

      // Focus first input in the new item if not readonly
      if (!this.readonly) {
        const firstInput = item.querySelector('input, textarea, select');
        if (firstInput) {
          firstInput.focus();
        }
      }
    }

    deleteItem(item) {
      if (this.disabled || this.readonly) return;

      const index = this.items.indexOf(item);
      if (index > -1) {
        this.items.splice(index, 1);
      }

      item.remove();
      this.updateHiddenInput();
      this.updateAddButtonVisibility();
    }

    setItemReadonly(item) {
      // Find all inputs, textareas, and selects in the item and set them to readonly/disabled
      const inputs = item.querySelectorAll('input:not([type="hidden"]), textarea, select');
      inputs.forEach(input => {
        if (input.tagName === 'SELECT') {
          input.disabled = true;
        } else {
          input.readOnly = true;
        }
      });
      
      // Hide the delete button
      const deleteButton = item.querySelector('[data-editable-list-item-delete]');
      if (deleteButton) {
        deleteButton.style.display = 'none';
      }
    }

    updateAddButtonVisibility() {
      if (!this.addButton) return;

      // Hide the add button if max items is reached
      if (this.maxItems > 0 && this.items.length >= this.maxItems) {
        this.addButton.style.display = 'none';
      } else {
        this.addButton.style.display = '';
      }
    }

    populateFromData(data) {
      // Clear existing items
      this.items = [];
      this.itemsContainer.innerHTML = '';

      if (Array.isArray(data)) {
        // Handle array data (simple list or list of objects)
        if (data.length === 0) {
          this.addItem();
          return;
        }

        data.forEach(itemData => {
          const item = this.createItemFromData(itemData);
          if (item) {
            this.itemsContainer.appendChild(item);
            this.items.push(item);
          }
        });
      } else if (typeof data === 'object' && data !== null) {
        // Handle object data (keyed by name input)
        const keys = Object.keys(data);
        if (keys.length === 0) {
          this.addItem();
          return;
        }

        keys.forEach(key => {
          const itemData = { _nameInputValue: key, ...data[key] };
          const item = this.createItemFromData(itemData);
          if (item) {
            this.itemsContainer.appendChild(item);
            this.items.push(item);
          }
        });
      } else {
        // Invalid data type, add empty item
        this.addItem();
      }

      this.updateHiddenInput();
    }

    createItemFromData(data) {
      // Clone the template content
      const templateContent = this.template.content.cloneNode(true);
      const item = templateContent.querySelector('[data-editable-list-item]');

      if (!item) {
        console.error('EditableList: No item found in template');
        return null;
      }

      // Generate unique ID for the new item
      item.id = 'list-item-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);

      // Get all inputs in the item
      const inputs = this.getItemInputs(item);

      // Populate inputs with data
      if (typeof data === 'string' || typeof data === 'number') {
        // Simple value - set to name input
        if (inputs.nameInput) {
          this.setInputValue(inputs.nameInput, data);
        }
      } else if (typeof data === 'object' && data !== null) {
        // Object value - set to corresponding named inputs
        if (data._nameInputValue && inputs.nameInput) {
          this.setInputValue(inputs.nameInput, data._nameInputValue);
        }

        inputs.others.forEach(input => {
          if (data.hasOwnProperty(input.name)) {
            this.setInputValue(input, data[input.name]);
          }
        });
      }

      // Apply readonly state if needed
      if (this.readonly) {
        this.setItemReadonly(item);
      }

      return item;
    }

    setInputValue(input, value) {
      if (input.type === 'checkbox') {
        input.checked = !!value;
      } else if (input.type === 'radio') {
        if (input.value === String(value)) {
          input.checked = true;
        }
      } else if (input.tagName === 'SELECT') {
        if (input.multiple) {
          // For multi-select, select multiple options
          const values = Array.isArray(value) ? value : [value];
          Array.from(input.options).forEach(option => {
            option.selected = values.includes(option.value);
          });
        } else {
          input.value = value;
        }
      } else {
        input.value = value;
      }
    }

    updateHiddenInput() {
      const data = this.aggregateData();
      this.hiddenInput.value = JSON.stringify(data);

      // Trigger validation
      if (this.required) {
        if (Array.isArray(data) && data.length === 0) {
          this.hiddenInput.setCustomValidity('At least one item is required');
        } else if (typeof data === 'object' && Object.keys(data).length === 0) {
          this.hiddenInput.setCustomValidity('At least one item is required');
        } else {
          this.hiddenInput.setCustomValidity('');
        }
      }
    }

    aggregateData() {
      const items = this.itemsContainer.querySelectorAll('[data-editable-list-item]');
      const results = [];
      let hasNameInput = false;
      let hasOtherInputs = false;

      // First pass: determine the structure
      items.forEach(item => {
        const inputs = this.getItemInputs(item);
        if (inputs.nameInput) {
          hasNameInput = true;
        }
        if (inputs.others.length > 0) {
          hasOtherInputs = true;
        }
      });

      // Second pass: aggregate data based on structure
      const resultMap = {};

      items.forEach(item => {
        const inputs = this.getItemInputs(item);
        
        if (hasNameInput && !hasOtherInputs) {
          // Case 1: Only name inputs - simple array
          if (inputs.nameInput && inputs.nameInput.value.trim() !== '') {
            results.push(inputs.nameInput.value.trim());
          }
        } else if (hasNameInput && hasOtherInputs) {
          // Case 4: Name input + other inputs - object keyed by name input
          if (inputs.nameInput && inputs.nameInput.value.trim() !== '') {
            const key = inputs.nameInput.value.trim();
            const obj = {};
            inputs.others.forEach(input => {
              obj[input.name] = this.getInputValue(input);
            });
            resultMap[key] = obj;
          }
        } else if (!hasNameInput && hasOtherInputs) {
          // Case 2 & 3: Other inputs only - array of objects
          const obj = {};
          let hasValue = false;
          inputs.others.forEach(input => {
            const value = this.getInputValue(input);
            obj[input.name] = value;
            if (value !== '' && value !== false) {
              hasValue = true;
            }
          });
          if (hasValue) {
            results.push(obj);
          }
        } else {
          // No valid inputs, skip
        }
      });

      // Return appropriate structure
      if (hasNameInput && hasOtherInputs) {
        return resultMap;
      } else {
        return results;
      }
    }

    getItemInputs(item) {
      const allInputs = item.querySelectorAll('input:not([type="hidden"]), textarea, select');
      const nameInput = Array.from(allInputs).find(input => input.name === '' || !input.name);
      const others = Array.from(allInputs).filter(input => input.name && input.name !== '');

      return { nameInput, others };
    }

    getInputValue(input) {
      if (input.type === 'checkbox') {
        return input.checked;
      } else if (input.type === 'radio') {
        const name = input.name;
        const checked = input.form ? 
          input.form.querySelector(`input[name="${name}"]:checked`) : 
          document.querySelector(`input[name="${name}"]:checked`);
        return checked ? checked.value : '';
      } else if (input.tagName === 'SELECT') {
        if (input.multiple) {
          // For multi-select, return array of selected values
          const selected = Array.from(input.selectedOptions).map(opt => opt.value);
          return selected.length > 0 ? selected : '';
        } else {
          return input.value;
        }
      } else if (input.type === 'number') {
        return input.value !== '' ? parseFloat(input.value) : '';
      } else {
        return input.value;
      }
    }
  }

  // Initialize on DOMContentLoaded
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initEditableLists);
  } else {
    initEditableLists();
  }

  // Re-initialize when new content is added (for dynamic content)
  if (window.MutationObserver) {
    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        mutation.addedNodes.forEach((node) => {
          if (node.nodeType === 1) { // Element node
            if (node.matches('[data-editable-list-name]')) {
              new EditableList(node);
            } else {
              const lists = node.querySelectorAll('[data-editable-list-name]');
              lists.forEach(list => {
                if (!list.dataset.editableListInitialized) {
                  new EditableList(list);
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
    module.exports = EditableList;
  }
})();
