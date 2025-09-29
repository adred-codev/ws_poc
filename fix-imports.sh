#!/bin/bash

# Fix all TypeScript import statements to include .js extension for ES modules

echo "Fixing import statements in TypeScript files..."

# Fix imports in all .ts files
find src -name "*.ts" -type f | while read -r file; do
    echo "Processing: $file"

    # Fix relative imports to add .js extension
    # Match patterns like: from './something' or from '../something'
    sed -i.bak -E "s/from '(\.\.[\/\.]*)([^']+)'/from '\1\2.js'/g" "$file"

    # Remove .js.js duplicates if any
    sed -i.bak -E "s/\.js\.js/\.js/g" "$file"

    # Remove backup files
    rm -f "${file}.bak"
done

echo "✅ Import statements fixed!"