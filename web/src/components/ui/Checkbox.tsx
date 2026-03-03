import React from 'react';
import styles from './Checkbox.module.css';

export interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: string;
}

export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ label, className, id, ...props }, ref) => {
    const checkboxId = id || `checkbox-${Math.random().toString(36).substring(2, 9)}`;

    return (
      <label className={`${styles.checkbox} ${className || ""}`} htmlFor={checkboxId}>
        <input
          ref={ref}
          type="checkbox"
          id={checkboxId}
          className={styles.input}
          {...props}
        />
        {label && <span className={styles.label}>{label}</span>}
      </label>
    );
  }
);

Checkbox.displayName = 'Checkbox';

export interface CheckboxGroupProps {
  label?: string;
  options: { value: string; label: string; checked?: boolean }[];
  onChange?: (values: string[]) => void;
  name?: string;
}

export function CheckboxGroup({ label, options, onChange, name }: CheckboxGroupProps) {
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (onChange) {
      const checkbox = e.target;
      const values = Array.from(
        document.querySelectorAll(`input[name="${name}"]:checked`)
      ).map((input) => (input as HTMLInputElement).value);

      // Add or remove the changed value
      if (checkbox.checked) {
        values.push(checkbox.value);
      } else {
        const index = values.indexOf(checkbox.value);
        if (index > -1) {
          values.splice(index, 1);
        }
      }

      onChange(values);
    }
  };

  return (
    <div className={styles.group}>
      {label && <div className={styles.groupLabel}>{label}</div>}
      <div className={styles.options}>
        {options.map((option) => (
          <Checkbox
            key={option.value}
            name={name}
            value={option.value}
            label={option.label}
            defaultChecked={option.checked}
            onChange={handleChange}
          />
        ))}
      </div>
    </div>
  );
}
